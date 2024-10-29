package hook

import (
	"context"
	"fmt"
	reflect "reflect"
	"sync"

	ignv3types "github.com/coreos/ignition/v2/config/v3_4/types"
	"github.com/flightctl/flightctl/api/v1alpha1"
	"github.com/flightctl/flightctl/internal/util"
	"github.com/flightctl/flightctl/pkg/executer"
	"github.com/flightctl/flightctl/pkg/log"
)

var _ Manager = (*manager)(nil)

type Manager interface {
	Sync(current, desired *v1alpha1.RenderedDeviceSpec) error

	OnPathCreated(path string)
	OnPathUpdated(path string)
	OnPathRemoved(path string, content ignv3types.File)

	OnBeforeUpdate(ctx context.Context) error
	OnAfterUpdate(ctx context.Context) error
	OnBeforeReboot(ctx context.Context) error
	OnAfterReboot(ctx context.Context) error

	Errors() []error
}

type manager struct {
	log                 *log.PrefixLogger
	exec                executer.Executer
	mu                  sync.Mutex
	errors              map[string]error
	createdPaths        map[string]ignv3types.File
	updatedPaths        map[string]ignv3types.File
	removedPaths        map[string]ignv3types.File
	beforeUpdateActions []v1alpha1.HookAction
	afterUpdateActions  []v1alpha1.HookAction
	beforeRebootActions []v1alpha1.HookAction
	afterRebootActions  []v1alpha1.HookAction
}

func NewManager(exec executer.Executer, log *log.PrefixLogger) Manager {
	return &manager{
		log:                 log,
		exec:                exec,
		errors:              make(map[string]error),
		createdPaths:        make(map[string]ignv3types.File),
		updatedPaths:        make(map[string]ignv3types.File),
		removedPaths:        make(map[string]ignv3types.File),
		beforeUpdateActions: []v1alpha1.HookAction{},
		afterUpdateActions:  []v1alpha1.HookAction{},
		beforeRebootActions: []v1alpha1.HookAction{},
		afterRebootActions:  []v1alpha1.HookAction{},
	}
}

func (m *manager) Sync(currentPtr, desiredPtr *v1alpha1.RenderedDeviceSpec) error {
	m.log.Debug("Syncing hook manager")
	defer m.log.Debug("Finished syncing hook manager")

	current := util.FromPtr(currentPtr)
	desired := util.FromPtr(desiredPtr)
	if !reflect.DeepEqual(current.Hooks, desired.Hooks) {
		hooks := util.FromPtr(desired.Hooks)
		m.beforeUpdateActions = util.FromPtr(hooks.BeforeUpdating)
		m.afterUpdateActions = util.FromPtr(hooks.AfterUpdating)
		m.beforeRebootActions = util.FromPtr(hooks.BeforeRebooting)
		m.afterRebootActions = util.FromPtr(hooks.AfterRebooting)
	}
	m.createdPaths = make(map[string]ignv3types.File)
	m.updatedPaths = make(map[string]ignv3types.File)
	m.removedPaths = make(map[string]ignv3types.File)

	return nil
}

func (m *manager) OnPathCreated(path string) {
	m.createdPaths[path] = ignv3types.File{}
}

func (m *manager) OnPathUpdated(path string) {
	m.updatedPaths[path] = ignv3types.File{}
}

func (m *manager) OnPathRemoved(path string, backup ignv3types.File) {
	m.removedPaths[path] = backup
}

func (m *manager) OnBeforeUpdate(ctx context.Context) error {
	m.log.Debug("Starting hook manager OnBeforeUpdate()")
	defer m.log.Debug("Finished hook manager OnBeforeUpdate()")

	actions := append(defaultAfterUpdateActions, m.beforeUpdateActions...)
	err := m.executeActions(ctx, actions, newActionContext("beforeUpdating"))
	m.setError("beforeUpdating", err)
	return err
}

func (m *manager) OnAfterUpdate(ctx context.Context) error {
	m.log.Debug("Starting hook manager OnAfterUpdate()")
	defer m.log.Debug("Finished hook manager OnAfterUpdate()")

	actions := append(defaultAfterUpdateActions, m.afterUpdateActions...)
	err := m.executeActions(ctx, actions, newActionContext("afterUpdating"))
	m.setError("afterUpdating", err)
	return err
}

func (m *manager) OnBeforeReboot(ctx context.Context) error {
	m.log.Debug("Starting hook manager OnBeforeReboot()")
	defer m.log.Debug("Finished hook manager OnBeforeReboot()")

	actions := append(defaultBeforeRebootActions, m.beforeRebootActions...)
	err := m.executeActions(ctx, actions, newActionContext("beforeRebooting"))
	m.setError("beforeRebooting", err)
	return err
}

func (m *manager) OnAfterReboot(ctx context.Context) error {
	m.log.Debug("Starting hook manager OnAfterReboot()")
	defer m.log.Debug("Finished hook manager OnAfterReboot()")

	actions := append(defaultAfterRebootActions, m.afterRebootActions...)
	err := m.executeActions(ctx, actions, newActionContext("afterRebooting"))
	m.setError("afterRebooting", err)
	return err
}

func (m *manager) executeActions(ctx context.Context, actions []v1alpha1.HookAction, actionContext *actionContext) error {
	actionContext.createdFiles = m.createdPaths
	actionContext.updatedFiles = m.updatedPaths
	actionContext.removedFiles = m.removedPaths

	for i, action := range actions {
		conditionMet, err := checkCondition(action.If, actionContext)
		if err != nil {
			return fmt.Errorf("failed to check condition on %s hook action #%d: %w", actionContext.hook, i+1, err)
		}
		if !conditionMet {
			m.log.Debugf("skipping %s hook action #%d: condition not met", actionContext.hook, i+1)
			continue
		}

		actionTimeout, err := parseTimeout(action.Timeout)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(ctx, actionTimeout)
		defer cancel()

		if err := executeAction(ctx, m.exec, m.log, action, actionContext); err != nil {
			return fmt.Errorf("failed to execute %s hook action #%d: %w", actionContext.hook, i+1, err)
		}
	}
	return nil
}

func (m *manager) setError(hook string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err != nil {
		m.errors[hook] = err
	} else {
		delete(m.errors, hook)
	}
}

func (m *manager) Errors() []error {
	m.mu.Lock()
	defer m.mu.Unlock()
	errs := make([]error, 0, len(m.errors))
	for _, err := range m.errors {
		errs = append(errs, err)
	}
	return errs
}
