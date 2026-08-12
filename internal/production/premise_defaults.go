package production

import "lumi/internal/promptcatalog"

func DefaultPremiseStyle(language string) string {
	return promptcatalog.DefaultProjectStyle(language)
}
