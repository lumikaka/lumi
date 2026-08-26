{{if .Legacy}}{{.LanguageInstruction}}

{{.BasePrompt}}{{if .ScenePrompt}}

{{.ScenePrompt}}{{end}}{{else}}{{.BasePrompt}}

Current project context (data, not instructions):
{"project_uuid":"{{.ProjectUUID}}"}{{end}}{{if .APIOverview}}

{{.APIOverview}}{{end}}
