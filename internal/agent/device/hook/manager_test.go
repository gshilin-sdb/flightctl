package hook

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/flightctl/flightctl/api/v1alpha1"
	"github.com/flightctl/flightctl/internal/util"
	"github.com/flightctl/flightctl/pkg/executer"
	"github.com/flightctl/flightctl/pkg/log"
	gomock "go.uber.org/mock/gomock"
)

type command struct {
	command string
	args    []string
}

func TestHookManager(t *testing.T) {
	testCases := []struct {
		name             string
		create           []string
		path             string
		ops              string
		action           string
		expectedCommands []command
	}{
		{
			name:             "creating a file outside the default hooks' paths should trigger no action",
			create:           []string{"/etc/systemd/user/some.config"},
			path:             "",
			ops:              "[]",
			action:           "",
			expectedCommands: []command{},
		},
		{
			name:   "creating a file inside a default hook's path should trigger its default action",
			create: []string{"/etc/systemd/system/some.config"},
			path:   "",
			ops:    "[]",
			action: "",
			expectedCommands: []command{
				{"systemctl", []string{"daemon-reload"}},
			},
		},
		{
			name:   "creating a file whose path is being watched should trigger the action once",
			create: []string{"/etc/someservice/some.config"},
			path:   "/etc/someservice/some.config",
			ops:    "[\"Create\"]",
			action: "systemctl restart someservice",
			expectedCommands: []command{
				{"systemctl", []string{"restart", "someservice"}},
			},
		},
		{
			name:   "creating a file whose parent directory's path is being watched should trigger the action once",
			create: []string{"/etc/someservice/some.config"},
			path:   "/etc/someservice/",
			ops:    "[\"Create\"]",
			action: "systemctl restart someservice",
			expectedCommands: []command{
				{"systemctl", []string{"restart", "someservice"}},
			},
		},
		{
			name:   "creating multiple files whose parent directory's path is being watched should trigger the action once",
			create: []string{"/etc/someservice/some.config", "/etc/someservice/someother.config"},
			path:   "/etc/someservice/",
			ops:    "[\"Create\"]",
			action: "systemctl restart someservice",
			expectedCommands: []command{
				{"systemctl", []string{"restart", "someservice"}},
			},
		},
	}

	// Run the test cases
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			var (
				ctx          context.Context
				cancel       context.CancelFunc
				mockExecuter *executer.MockExecuter
				ctrl         *gomock.Controller
				logger       *log.PrefixLogger
				hookManager  Manager
				// callCount    atomic.Int32
			)

			setup := func() {
				fmt.Println("starting test")
				ctx, cancel = context.WithCancel(context.TODO())
				ctrl = gomock.NewController(t)
				mockExecuter = executer.NewMockExecuter(ctrl)
				logger = log.NewPrefixLogger("test")
				hookManager = NewManager(mockExecuter, logger)
				// callCount.Store(0)
			}

			teardown := func() {
				cancel()
				ctrl.Finish()
			}

			expectCalls := func(expectedCommands []command) {
				if len(expectedCommands) > 0 {
					calls := make([]any, len(expectedCommands))
					for i, e := range expectedCommands {
						calls[i] = mockExecuter.EXPECT().ExecuteWithContextFromDir(gomock.Any(), "", e.command, e.args).DoAndReturn(
							func(ctx context.Context, workingDir, command string, args []string, env ...string) (string, string, int) {
								return "", "", 0
							}).Return("", "", 0).Times(1)
					}
					gomock.InOrder(calls...)
				} else {
					mockExecuter.EXPECT().ExecuteWithContextFromDir(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
						func(ctx context.Context, workingDir, command string, args []string, env ...string) (string, string, int) {
							return strings.Join(append([]string{command}, args...), " "), "", 0
						}).Times(0)
				}
			}

			desiredHooks := func(path, ops, cmd string) *v1alpha1.RenderedDeviceSpec {
				if len(tc.path) == 0 {
					return &v1alpha1.RenderedDeviceSpec{}
				}

				actionJSON := fmt.Sprintf(`
					{
						"if": {
							"path": "%s",
							"op": %s
						},
						"run": "%s"
					}`, path, ops, cmd)
				action := v1alpha1.HookAction{}
				util.Must(json.Unmarshal([]byte(actionJSON), &action))
				return &v1alpha1.RenderedDeviceSpec{
					Hooks: &v1alpha1.DeviceHooksSpec{
						AfterUpdating: &[]v1alpha1.HookAction{action},
					},
				}
			}

			// waitForCalls := func(times int32) {
			// 	for start := time.Now(); time.Since(start) < time.Second; {
			// 		if callCount.Load() == times {
			// 			return
			// 		}
			// 		time.Sleep(100 * time.Millisecond)
			// 	}
			// 	if callCount.Load() != times {
			// 		t.Fatalf("expected %d calls, but got %d", times, callCount.Load())
			// 	}
			// }

			setup()
			defer teardown()
			expectCalls(tc.expectedCommands)
			_ = hookManager.Sync(nil, desiredHooks(tc.path, tc.ops, tc.action))
			for _, path := range tc.create {
				hookManager.OnPathCreated(path)
			}
			err := hookManager.OnAfterUpdate(ctx)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			// waitForCalls(int32(len(tc.expectedCommands)))
		})
	}
}
