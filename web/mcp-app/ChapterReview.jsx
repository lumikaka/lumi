import { useQuery } from '@tanstack/react-query'
import { Link, useParams } from 'react-router-dom'
import { useI18n } from '../src/i18n/useI18n.js'
import { useWorkspaceRuntime } from '../src/workspace/runtime.jsx'
import { SimplePagesPage } from '../src/pages/SimpleProjectPages.jsx'
import SectionRail from '../src/components/book-preview/SectionRail.jsx'
import { webBookLabels, webBookPages } from '../src/components/book-preview/webAdapter.js'
import { orderedSimplePages } from '../src/pages/simpleProjectState.js'

// Additional MCP chapter structure surrounds the unchanged Web page manager.
export default function ChapterReview({ project, projectUuid }) {
  const { chapterUuid } = useParams()
  const { t } = useI18n()
  const { api, loadImage } = useWorkspaceRuntime()
  const chapter = useQuery({ queryKey: ['story-chapter', projectUuid, chapterUuid], queryFn: () => api.getChapter(projectUuid, chapterUuid) })
  const sections = useQuery({ queryKey: ['comic-sections', projectUuid, chapterUuid, 'summary'], queryFn: () => api.listComicSectionSummaries(projectUuid, chapterUuid) })
  const pages = webBookPages(orderedSimplePages(sections.data?.items || []), t)
  return <div className="review-workspace"><SectionRail title={t('simple.pages.title')} pages={pages} loadImage={loadImage} labels={webBookLabels(t)} renderLink={(page, props, children) => <Link {...props} to={`/projects/${projectUuid}/chapters/${chapterUuid}/sections/${page.uuid}`}>{children}</Link>} /><div><section className="simple-project-page"><h2>{t('simple.chapter.story')}</h2><p className="simple-review-story" data-user-content>{chapter.data?.current_story?.content || t('story.chapter.placeholder')}</p></section><SimplePagesPage project={project} projectUuid={projectUuid} /></div></div>
}
