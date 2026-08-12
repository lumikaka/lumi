import { BookOpenText, GalleryVertical, Image, MousePointerClick, PanelsTopLeft } from 'lucide-react'
import { useI18n } from '../i18n/useI18n.js'
import {
  ASPECT_RATIO_MODES,
  COMIC_LAYOUTS,
  INTERACTION_MODES,
  PICTURE_BOOK_FORMATS,
  draftForPictureBookFormat,
  pictureBookAspectKey,
  pictureBookDraftIsValid,
  pictureBookFormatKey,
} from '../pages/pictureBookProfile.js'

const ASPECT_FORMATS = new Set(['classic_picture_book', 'wordless_picture_book', 'comic_story'])
const FORMAT_ICONS = {
  classic_picture_book: BookOpenText,
  wordless_picture_book: Image,
  interactive_picture_book: MousePointerClick,
  comic_story: PanelsTopLeft,
  vertical_strip: GalleryVertical,
}

export default function PictureBookProfileFields({ value, onChange }) {
  const { t } = useI18n()
  const set = (changes) => onChange({ ...value, ...changes })
  const customInvalid = value.aspectMode === 'custom' && !pictureBookDraftIsValid(value)
  return (
    <section className="picture-book-fields" aria-labelledby="picture-book-format-label">
      <div className="picture-book-fields__heading">
        <h3 id="picture-book-format-label">{t('projects.picture_book.title')}</h3>
        <p>{t('projects.picture_book.immutable_hint')}</p>
      </div>
      <div className="picture-book-format-cards" role="radiogroup" aria-labelledby="picture-book-format-label">
        {PICTURE_BOOK_FORMATS.map((format) => {
          const FormatIcon = FORMAT_ICONS[format]
          return (
            <button key={format} type="button" role="radio" aria-checked={value.format === format} aria-pressed={value.format === format} onClick={() => onChange(draftForPictureBookFormat(format))}>
              <FormatIcon className="picture-book-format-card__icon" size={22} strokeWidth={1.8} aria-hidden="true" />
              <span className="picture-book-format-card__copy">
                <strong>{t(pictureBookFormatKey(format))}</strong>
                <span>{t(`${pictureBookFormatKey(format)}.description`)}</span>
              </span>
            </button>
          )
        })}
      </div>

      {ASPECT_FORMATS.has(value.format) ? (
        <fieldset className="picture-book-options">
          <legend>{t('projects.picture_book.field.aspect_ratio')}</legend>
          <div className="picture-book-choice-row">
            {ASPECT_RATIO_MODES.map((mode) => <button key={mode} type="button" aria-pressed={value.aspectMode === mode} onClick={() => set({ aspectMode: mode })}>{t(pictureBookAspectKey(mode))}</button>)}
          </div>
          {value.aspectMode === 'custom' ? (
            <div className="picture-book-custom-ratio">
              <label>{t('projects.picture_book.custom.width')}<input type="number" min="1" max="100" step="1" required value={value.customWidth} onChange={(event) => set({ customWidth: event.target.valueAsNumber })} /></label>
              <span aria-hidden="true">:</span>
              <label>{t('projects.picture_book.custom.height')}<input type="number" min="1" max="100" step="1" required value={value.customHeight} onChange={(event) => set({ customHeight: event.target.valueAsNumber })} /></label>
              {customInvalid ? <p role="alert">{t('projects.picture_book.custom.invalid')}</p> : <p>{t('projects.picture_book.custom.hint')}</p>}
            </div>
          ) : null}
        </fieldset>
      ) : null}

      {value.format === 'classic_picture_book' ? <label className="picture-book-check"><input type="checkbox" checked={value.largeImageMinimalText} onChange={(event) => set({ largeImageMinimalText: event.target.checked })} /><span><strong>{t('projects.picture_book.field.large_image_minimal_text')}</strong><small>{t('projects.picture_book.large_image_minimal_text_hint')}</small></span></label> : null}

      {value.format === 'interactive_picture_book' ? <label className="picture-book-select">{t('projects.picture_book.field.interaction_mode')}<select value={value.interactionMode} onChange={(event) => set({ interactionMode: event.target.value })}>{INTERACTION_MODES.map((mode) => <option value={mode} key={mode}>{t(`projects.picture_book.interaction.${mode}`)}</option>)}</select></label> : null}

      {value.format === 'comic_story' ? <fieldset className="picture-book-options"><legend>{t('projects.picture_book.field.comic_layout')}</legend><div className="picture-book-choice-row">{COMIC_LAYOUTS.map((layout) => <button key={layout} type="button" aria-pressed={value.comicLayout === layout} onClick={() => set({ comicLayout: layout })}>{t(`projects.picture_book.comic_layout.${layout}`)}</button>)}</div></fieldset> : null}
    </section>
  )
}
