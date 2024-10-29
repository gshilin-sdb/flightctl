package hook

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	ignv3types "github.com/coreos/ignition/v2/config/v3_4/types"
	"github.com/flightctl/flightctl/api/v1alpha1"
	"github.com/flightctl/flightctl/internal/util"
	"github.com/flightctl/flightctl/pkg/executer"
	"github.com/flightctl/flightctl/pkg/log"
)

type CommandLineVarKey string

const (
	DefaultHookActionTimeout = 10 * time.Second

	// PathKey defines the name of the variable that contains the path operated on
	PathKey CommandLineVarKey = "Path"
	// FilesKey defines the name of the variable that contains a space-
	// separated list of files created, updated, or removed during the update
	FilesKey CommandLineVarKey = "Files"
	// CreatedKey defines the name of the variable that contains a space-
	// separated list of files created during the update
	CreatedKey CommandLineVarKey = "CreatedFiles"
	// UpdatedKey defines the name of the variable that contains a space-
	// separated list of files updated during the update
	UpdatedKey CommandLineVarKey = "UpdatedFiles"
	// RemovedKey defines the name of the variable that contains a space-
	// separated list of files removed during the update
	RemovedKey CommandLineVarKey = "RemovedFiles"
	// BackupKey defines the name of the variable that contains a space-
	// separated list of files backed up before removal from the system
	// into a temporary location deleted after the action completes.
	BackupKey CommandLineVarKey = "BackupFiles"

	leftDelim     = `{{`
	rightDelim    = `}}`
	optWhitespace = `\s*`
)

var (
	matchers = map[CommandLineVarKey]*regexp.Regexp{
		PathKey:    regexp.MustCompile(leftDelim + optWhitespace + string(PathKey) + optWhitespace + rightDelim),
		FilesKey:   regexp.MustCompile(leftDelim + optWhitespace + string(FilesKey) + optWhitespace + rightDelim),
		CreatedKey: regexp.MustCompile(leftDelim + optWhitespace + string(CreatedKey) + optWhitespace + rightDelim),
		UpdatedKey: regexp.MustCompile(leftDelim + optWhitespace + string(UpdatedKey) + optWhitespace + rightDelim),
		RemovedKey: regexp.MustCompile(leftDelim + optWhitespace + string(RemovedKey) + optWhitespace + rightDelim),
	}
)

type actionContext struct {
	hook            string
	createdFiles    map[string]ignv3types.File
	updatedFiles    map[string]ignv3types.File
	removedFiles    map[string]ignv3types.File
	commandLineVars map[CommandLineVarKey]string
}

func newActionContext(hook string) *actionContext {
	return &actionContext{
		hook:            hook,
		commandLineVars: make(map[CommandLineVarKey]string),
	}
}

func resetCommandLineVars(actionCtx *actionContext) {
	for key := range actionCtx.commandLineVars {
		actionCtx.commandLineVars[key] = ""
	}
}

func executeAction(ctx context.Context, exec executer.Executer, log *log.PrefixLogger, action v1alpha1.HookAction, actionContext *actionContext) error {
	actionType, err := action.Type()
	if err != nil {
		return err
	}

	switch actionType {
	case v1alpha1.RunActionType:
		runAction, err := action.AsHookActionRun()
		if err != nil {
			return err
		}
		return executeRunAction(ctx, exec, log, runAction, actionContext)
	default:
		return fmt.Errorf("unknown hook action type %q", actionType)
	}
}

func executeRunAction(ctx context.Context, exec executer.Executer, log *log.PrefixLogger,
	action v1alpha1.HookActionRun, actionCtx *actionContext) error {

	var workDir string
	if action.WorkDir != nil {
		workDir = *action.WorkDir
		dirExists, err := dirExists(workDir)
		if err != nil {
			return err
		}

		// we expect the directory to exist should be created by config if its new.
		if !dirExists {
			return fmt.Errorf("workdir %s: %w", workDir, os.ErrNotExist)
		}
	}

	// render variables in args if they exist
	commandLine := replaceTokens(action.Run, actionCtx.commandLineVars)
	cmd, args := splitCommandAndArgs(commandLine)

	if err := validateEnvVars(action.EnvVars); err != nil {
		return err
	}
	envVars := util.LabelMapToArray(action.EnvVars)

	_, stderr, exitCode := exec.ExecuteWithContextFromDir(ctx, workDir, cmd, args, envVars...)
	if exitCode != 0 {
		log.Errorf("running %q returned with exit code %d: %s", commandLine, exitCode, stderr)
		return fmt.Errorf("%s (exit code %d)", stderr, exitCode)
	}

	return nil
}

func dirExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if err == nil {
		return info.IsDir(), nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("failed to check if directory %s exists: %w", path, err)
}

func parseTimeout(timeout *string) (time.Duration, error) {
	if timeout == nil {
		return DefaultHookActionTimeout, nil
	}
	return time.ParseDuration(*timeout)
}

func splitCommandAndArgs(command string) (string, []string) {
	parts := splitWithQuotes(command)
	if len(parts) == 0 {
		return "", []string{}
	}
	return parts[0], parts[1:]
}

func splitWithQuotes(s string) []string {
	quoted := false
	return strings.FieldsFunc(s, func(r rune) bool {
		if r == '"' {
			quoted = !quoted
		}
		return !quoted && r == ' '
	})
}

func validateEnvVars(envVars *map[string]string) error {
	if envVars == nil {
		return nil
	}
	for key, value := range *envVars {
		if key == "" {
			return fmt.Errorf("invalid envVar format: key cannot be empty: %s", strings.Join([]string{key, value}, "="))
		}
		if strings.Contains(key, " ") {
			return fmt.Errorf("invalid envVar format: key cannot contain spaces: %s", strings.Join([]string{key, value}, "="))
		}
		if value == "" {
			return fmt.Errorf("invalid envVar format: value cannot be empty: %s", strings.Join([]string{key, value}, "="))
		}
		if key != strings.ToUpper(key) {
			return fmt.Errorf("invalid envVar format: key must be uppercase: %s", strings.Join([]string{key, value}, "="))
		}
	}
	return nil
}

// replaceTokens replaces all registered command line variables with the
// provided values. Wrongly formatted or unknown variables are left in
// in the string.
func replaceTokens(s string, tokens map[CommandLineVarKey]string) string {
	for key, re := range matchers {
		s = re.ReplaceAllString(s, tokens[key])
	}
	return s
}
