import { useQuery } from '@tanstack/react-query'
import { previewProjectDirectoryName } from '../api/projects.js'
import LocalizedErrorMessage from '../i18n/LocalizedErrorMessage.jsx'
import { useI18n } from '../i18n/useI18n.js'

export function useProjectDirectoryPreview(projectUuid, name, enabled) {
  return useQuery({
    queryKey: ['project-directory-preview', projectUuid, name.trim()],
    queryFn: () => previewProjectDirectoryName(projectUuid, name.trim()),
    enabled: Boolean(enabled && projectUuid && name.trim()),
    retry: false,
    gcTime: 0,
  })
}

export default function ProjectDirectoryRenameOption({ checked, onChange, preview, disabled = false }) {
  const { t } = useI18n()
  return (
    <div className="project-directory-rename">
      <label className="project-directory-rename__toggle">
        <input type="checkbox" checked={checked} disabled={disabled} onChange={(event) => onChange(event.target.checked)} />
        <span>{t('projects.rename.directory')}</span>
      </label>
      {checked ? <>
        <p className="project-dialog-hint">{t('projects.rename.path_preview')}</p>
        {preview.isPending ? <p role="status">{t('projects.rename.preview_loading')}</p> : null}
        {preview.data ? <code className="project-directory-rename__path" data-no-i18n>{preview.data.root_path}</code> : null}
        <LocalizedErrorMessage error={preview.error} compact />
        <p className="project-dialog-hint">{t('projects.rename.directory_hint')}</p>
      </> : null}
    </div>
  )
}
