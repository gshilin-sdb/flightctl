package hook

import (
	"encoding/json"

	"github.com/flightctl/flightctl/api/v1alpha1"
	"github.com/flightctl/flightctl/internal/util"
)

const (
	defaultBeforeUpdateActionsJSON = `[]`
	defaultAfterUpdateActionsJSON  = `[
		{
			"if": {
				"path": "/etc/systemd/system/",
				"op": ["Create", "Update", "Remove"]
			},
			"run": "systemctl daemon-reload"
		},
		{
			"if": {
				"path": "/etc/NetworkManager/system-connections/",
				"op": ["Create", "Update", "Remove"]
			},
			"run": "nmcli conn reload"
		},
		{
			"if": {
				"path": "/etc/firewalld/",
				"op": ["Create", "Update", "Remove"]
			},
			"run": "firewall-cmd --reload"
		}
	]`
	defaultBeforeRebootActionsJSON = `[]`
	defaultAfterRebootActionsJSON  = `[]`
)

var (
	defaultBeforeUpdateActions = []v1alpha1.HookAction{}
	defaultAfterUpdateActions  = []v1alpha1.HookAction{}
	defaultBeforeRebootActions = []v1alpha1.HookAction{}
	defaultAfterRebootActions  = []v1alpha1.HookAction{}
)

func init() {
	util.Must(json.Unmarshal([]byte(defaultBeforeUpdateActionsJSON), &defaultBeforeUpdateActions))
	util.Must(json.Unmarshal([]byte(defaultAfterUpdateActionsJSON), &defaultAfterUpdateActions))
	util.Must(json.Unmarshal([]byte(defaultBeforeRebootActionsJSON), &defaultBeforeRebootActions))
	util.Must(json.Unmarshal([]byte(defaultAfterRebootActionsJSON), &defaultAfterRebootActions))
}
