import { useId, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { X } from 'lucide-react'
import { ensureProjectOpen } from '../api/projects.js'
import { getStoryProject, updateStoryProject } from '../api/story.js'
import { useI18n } from '../i18n/useI18n.js'
import LocalizedErrorMessage from '../i18n/LocalizedErrorMessage.jsx'
import LumiDialog from './LumiDialog.jsx'
import ProjectDirectoryRenameOption, { useProjectDirectoryPreview } from './ProjectDirectoryRenameOption.jsx'

export default function ProjectRenameDialog({ project, onClose }) {
  const { t } = useI18n()
  const titleId = useId()
  const queryClient = useQueryClient()
  const [name, setName] = useState(project.name)
  const [renameDirectory, setRenameDirectory] = useState(false)
  const detail = useQuery({
    queryKey: ['story-project', project.uuid],
    queryFn: async () => {
      await ensureProjectOpen(project.uuid)
      return getStoryProject(project.uuid)
    },
    retry: false,
  })
  const preview = useProjectDirectoryPreview(project.uuid, name, renameDirectory && Boolean(detail.data))
  const save = useMutation({
    mutationFn: () => updateStoryProject(project.uuid, {
      name: name.trim(),
      description: detail.data.description,
      expected_revision: detail.data.revision,
      rename_directory: renameDirectory,
    }),
    onSuccess: (updated) => {
      queryClient.setQueryData(['story-project', project.uuid], updated)
      queryClient.invalidateQueries({ queryKey: ['recent-projects'] })
      queryClient.invalidateQueries({ queryKey: ['open-projects'] })
      onClose()
    },
  })
  const canSave = Boolean(name.trim() && detail.data && !detail.isFetching && !detail.error && !save.isPending && (!renameDirectory || (preview.data && !preview.isFetching && !preview.error)))
  return (
    <LumiDialog aria-labelledby={titleId} dismissDisabled={save.isPending} onClose={onClose}>
      <header className="lumi-dialog__header">
        <h2 id={titleId}>{t('projects.action.rename')}</h2>
        <button className="button-quiet" type="button" disabled={save.isPending} aria-label={t('common.action.close')} onClick={onClose}><X size={17} aria-hidden="true" /></button>
      </header>
      <div className="lumi-dialog__body">
        <form className="project-dialog-form" onSubmit={(event) => { event.preventDefault(); if (canSave) save.mutate() }}>
          <label>{t('projects.field.name')}<input value={name} onChange={(event) => { setName(event.target.value); save.reset() }} required maxLength={120} autoFocus disabled={save.isPending} /></label>
          <ProjectDirectoryRenameOption checked={renameDirectory} onChange={setRenameDirectory} preview={preview} disabled={save.isPending} />
          <LocalizedErrorMessage error={save.error || detail.error} compact />
          <div className="lumi-dialog__actions">
            <button className="button-secondary" type="button" disabled={save.isPending} onClick={onClose}>{t('common.action.cancel')}</button>
            <button type="submit" disabled={!canSave}>{t(save.isPending ? 'common.status.saving' : 'common.action.save')}</button>
          </div>
        </form>
      </div>
    </LumiDialog>
  )
}
