import { BookPreviewImage } from './BookPreview.jsx'
import './section-rail.scss'

// Transport-free section navigation shared by Web chapter structure and MCP reviews.
export default function SectionRail({ pages, currentUUID, title, overview, renderLink, loadImage, labels }) {
  return <aside className="review-section-rail" aria-label={title}>
    <h2>{title}</h2>{overview}
    <div className="review-section-rail__list">{pages.map(page => <div className="review-section-rail__row" key={page.uuid}>
      {renderLink(page, railLinkProps(page, currentUUID), <>
        <span className="review-section-rail__image"><BookPreviewImage key={`${page.uuid}:${page.image_key || 'missing'}`} page={page} loadImage={loadImage} labels={labels} /></span>
        <span><small>{page.label}</small><strong>{page.title}</strong></span>
      </>)}
    </div>)}</div>
  </aside>
}

function railLinkProps(page, currentUUID) {
  return { className: 'review-section-rail__link', 'aria-current': page.uuid === currentUUID ? 'page' : undefined, 'aria-label': `${page.label} · ${page.title}` }
}
