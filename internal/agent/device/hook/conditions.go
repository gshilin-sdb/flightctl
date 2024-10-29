package hook

import (
	"fmt"
	"slices"
	"strings"

	ignv3types "github.com/coreos/ignition/v2/config/v3_4/types"
	"github.com/flightctl/flightctl/api/v1alpha1"
)

func checkCondition(cond *v1alpha1.HookAction_If, actionContext *actionContext) (bool, error) {
	if cond == nil {
		return true, nil
	}

	conditionType, err := (*cond).Type()
	if err != nil {
		return false, err
	}

	switch conditionType {
	case v1alpha1.FileOpConditionType:
		fileOpCondition, err := (*cond).AsHookConditionFileOp()
		if err != nil {
			return false, err
		}
		return checkFileOpCondition(fileOpCondition, actionContext), nil
	default:
		return false, fmt.Errorf("unknown hook condition type %q", conditionType)
	}
}

func checkFileOpCondition(cond v1alpha1.HookConditionFileOp, actionCtx *actionContext) bool {
	resetCommandLineVars(actionCtx)

	isPathToDir := len(cond.Path) > 0 && cond.Path[len(cond.Path)-1] == '/'
	if isPathToDir {
		return checkFileOpConditionForDir(cond, actionCtx)
	} else {
		return checkFileOpConditionForFile(cond, actionCtx)
	}
}

// checkFileOpConditionForDir checks whether a specified operation (create, update, remove) has been performed
// on any file in the tree rooted at the specified path.
// As a side-effect, it populates the command line variables of the action context with the corresponding list of files.
func checkFileOpConditionForDir(cond v1alpha1.HookConditionFileOp, actionCtx *actionContext) bool {
	// ensure dir paths end with a trailing slash, so we don't accidentally match a file with a similar prefix
	if len(cond.Path) < 1 || cond.Path[len(cond.Path)-1] != '/' {
		cond.Path += "/"
	}

	conditionMet := false
	if slices.Contains(cond.Op, v1alpha1.FileOperationCreate) {
		if treeFromPathContains(cond.Path, actionCtx.createdFiles) {
			files := getContainedFiles(cond.Path, actionCtx.createdFiles)
			appendFiles(actionCtx, FilesKey, files...)
			appendFiles(actionCtx, CreatedKey, files...)
			conditionMet = true
		}
	}
	if slices.Contains(cond.Op, v1alpha1.FileOperationUpdate) {
		if treeFromPathContains(cond.Path, actionCtx.updatedFiles) {
			files := getContainedFiles(cond.Path, actionCtx.updatedFiles)
			appendFiles(actionCtx, FilesKey, files...)
			appendFiles(actionCtx, UpdatedKey, files...)
			conditionMet = true
		}
	}
	if slices.Contains(cond.Op, v1alpha1.FileOperationRemove) {
		if treeFromPathContains(cond.Path, actionCtx.removedFiles) {
			files := getContainedFiles(cond.Path, actionCtx.removedFiles)
			appendFiles(actionCtx, FilesKey, files...)
			appendFiles(actionCtx, RemovedKey, files...)
			conditionMet = true
		}
	}
	if conditionMet {
		actionCtx.commandLineVars[PathKey] = cond.Path
	}
	return conditionMet
}

// checkFileOpConditionForFile checks whether a specified operation (create, update, remove) has been performed
// on the specified file.
// As a side-effect, it populates the command line variables of the action context with the corresponding list of files.
func checkFileOpConditionForFile(cond v1alpha1.HookConditionFileOp, actionCtx *actionContext) bool {
	conditionMet := false
	if slices.Contains(cond.Op, v1alpha1.FileOperationCreate) {
		if pathEquals(cond.Path, actionCtx.createdFiles) {
			appendFiles(actionCtx, FilesKey, cond.Path)
			appendFiles(actionCtx, CreatedKey, cond.Path)
			conditionMet = true
		}
	}
	if slices.Contains(cond.Op, v1alpha1.FileOperationUpdate) {
		if pathEquals(cond.Path, actionCtx.updatedFiles) {
			appendFiles(actionCtx, FilesKey, cond.Path)
			appendFiles(actionCtx, UpdatedKey, cond.Path)
			conditionMet = true
		}
	}
	if slices.Contains(cond.Op, v1alpha1.FileOperationRemove) {
		if pathEquals(cond.Path, actionCtx.removedFiles) {
			appendFiles(actionCtx, FilesKey, cond.Path)
			appendFiles(actionCtx, RemovedKey, cond.Path)
			conditionMet = true
		}
	}
	if conditionMet {
		actionCtx.commandLineVars[PathKey] = cond.Path
	}
	return conditionMet
}

func pathEquals(path string, files map[string]ignv3types.File) bool {
	_, ok := files[path]
	return ok
}

func treeFromPathContains(path string, files map[string]ignv3types.File) bool {
	for file := range files {
		if strings.HasPrefix(file, path) {
			return true
		}
	}
	return false
}

func getContainedFiles(path string, files map[string]ignv3types.File) []string {
	containedFiles := []string{}
	for file := range files {
		if strings.HasPrefix(file, path) {
			containedFiles = append(containedFiles, file)
		}
	}
	return containedFiles
}

func appendFiles(actionCtx *actionContext, key CommandLineVarKey, files ...string) {
	actionCtx.commandLineVars[key] = strings.Join(append([]string{actionCtx.commandLineVars[key]}, files...), " ")
}
