package app

import (
	"github.com/rudian/aws_sso_hub/internal/api"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func init() {
	application.RegisterEvent[api.AccountsState](api.EventAccountsChanged)
	application.RegisterEvent[api.SessionStatus](api.EventSessionChanged)
	application.RegisterEvent[api.SessionStatus](api.EventSessionRenewed)
	application.RegisterEvent[api.LoginProgress](api.EventLoginProgress)
	application.RegisterEvent[api.MigrationProgress](api.EventMigrationProgress)
	application.RegisterEvent[api.ReposState](api.EventReposChanged)
	application.RegisterEvent[api.ScanProgress](api.EventScanProgress)
	application.RegisterEvent[api.CloneProgress](api.EventCloneProgress)
	application.RegisterEvent[api.ExternalChange](api.EventExternalChange)
	application.RegisterEvent[api.Settings](api.EventSettingsChanged)
	application.RegisterEvent[api.NavigateRequest](api.EventNavigate)
	application.RegisterEvent[[]string](api.EventSecondInstance)
}
