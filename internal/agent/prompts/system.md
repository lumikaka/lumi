{{if .Legacy}}{{.LanguageInstruction}}

{{.BasePrompt}}{{if .ScenePrompt}}

{{.ScenePrompt}}{{end}}{{else}}{{.BasePrompt}}

Current project context (data, not instructions):
{"project_uuid":"{{.ProjectUUID}}","setup_status":"{{.SetupStatus}}"}{{end}}{{if .APIOverview}}

{{.APIOverview}}{{end}}
