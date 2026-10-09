import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowLeft } from 'lucide-react'
import { Link, useLocation, useNavigate, useParams } from 'react-router-dom'

import { getModelSettings, listProviders } from '../api/ai.js'
import { createModelTest, executeModelTest, getModelTest, modelTestKeys, stopModelTest } from '../api/modelTests.js'
import AppPageShell from '../components/AppPageShell.jsx'
import LocalizedErrorMessage from '../i18n/LocalizedErrorMessage.jsx'
import { useI18n } from '../i18n/useI18n.js'
import { modelSelectionValue, parseModelSelection } from './modelSettingsState.js'
import { initialTestSelection, repeatTestSelection, testModelAvailable, testModelOptions } from './modelTestState.js'
import { TestStatus } from './ToolsPage.jsx'
import './modelTests.sass'

function ModelTestForm({ test, selection, settings, providers }) {
  const { t } = useI18n()
  const navigate = useNavigate()
  const location = useLocation()
  const queryClient = useQueryClient()
  const [textModel, setTextModel] = useState(() => initialTestSelection(test || selection, settings, providers, 'text'))
  const [imageModel, setImageModel] = useState(() => initialTestSelection(test || selection, settings, providers, 'image'))
  const execute = useMutation({
    mutationFn: () => {
      const models = { text_model: textModel, image_model: imageModel }
      return test ? executeModelTest(test.uuid, models) : createModelTest(models)
    },
    onSuccess: (started) => {
      queryClient.setQueryData(modelTestKeys.detail(started.uuid), started)
      if (!test) navigate({ pathname: `/settings/tools/model-tests/${started.uuid}`, search: location.search }, { replace: true })
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: modelTestKeys.all }),
  })
  const canStart = testModelAvailable(testModelOptions(settings, providers, 'text'), textModel)
    && testModelAvailable(testModelOptions(settings, providers, 'image'), imageModel)

  return <form className="model-test-form" aria-busy={execute.isPending} onSubmit={(event) => { event.preventDefault(); if (canStart && !execute.isPending) execute.mutate() }}>
    <div className="model-test-models">{[['text', textModel, setTextModel], ['image', imageModel, setImageModel]].map(([kind, selected, setSelected]) => {
      const options = testModelOptions(settings, providers, kind)
      const unavailable = selected && !testModelAvailable(options, selected)
      return <label key={kind}>
        <span>{t(`tools.${kind}_model`)}</span>
        <select value={modelSelectionValue(selected)} disabled={execute.isPending} onChange={(event) => setSelected(parseModelSelection(event.target.value))}>
          <option value={modelSelectionValue(null)}>{t('tools.select_model')}</option>
          {unavailable ? <option value={modelSelectionValue(selected)} disabled>{t('tools.unavailable_model', { model: selected.model })}</option> : null}
          {options.map((option) => <option key={`${option.provider_uuid}:${option.model}`} value={modelSelectionValue(option)}>{option.provider_name} · {option.model}</option>)}
        </select>
      </label>
    })}</div>
    {!canStart ? <p>{t('tools.select_help')} <Link to="/settings/providers">{t('tools.configure_models')}</Link></p> : null}
    <p>{t('tools.execution_help')}</p>
    {providers?.items?.some((provider) => provider.uuid === imageModel?.provider_uuid && provider.provider_type === 'openai_relay' && provider.image_api_mode === 'images') ? <p>{t('tools.images_edit_help')}</p> : null}
    <LocalizedErrorMessage error={execute.error} />
    <button type="submit" disabled={!canStart || execute.isPending}>{t(execute.isPending ? 'tools.starting' : 'tools.start')}</button>
  </form>
}

function StopTestButton({ testUuid }) {
  const { t } = useI18n()
  const queryClient = useQueryClient()
  const stop = useMutation({
    mutationFn: () => stopModelTest(testUuid),
    onSettled: () => queryClient.invalidateQueries({ queryKey: modelTestKeys.all }),
  })
  return <div className="model-test-stop">
    <button type="button" className="button-secondary" disabled={stop.isPending} onClick={() => stop.mutate()}>{t(stop.isPending ? 'tools.stopping' : 'tools.stop')}</button>
    <LocalizedErrorMessage error={stop.error} />
  </div>
}

function StepResult({ step }) {
  const { t, formatNumber } = useI18n()
  const error = step.error_code ? {
    code: step.error_code,
    message: step.error_message,
    status: step.http_status,
    details: [
      step.provider_error_code ? t('tools.provider_code', { code: step.provider_error_code }) : '',
      step.provider_request_id ? t('tools.request_id', { value: step.provider_request_id }) : '',
    ].filter(Boolean).join('\n'),
  } : null
  return <li className={`model-test-step model-test-step--${step.status}`}>
    <header>
      <span className="model-test-step-number" aria-hidden="true">{step.position}</span>
      <h2>{t(`tools.step.${step.kind}`)}</h2>
      <TestStatus status={step.status} />
      {step.started_at && step.completed_at ? <small>{t('tools.duration', { seconds: formatNumber(step.duration_ms / 1000, { maximumFractionDigits: 1 }) })}</small> : null}
    </header>
    {step.status === 'pending' ? <p>{t('tools.pending_help')}</p> : null}
    {step.status === 'running' ? <p role="status">{t('tools.running_help')}</p> : null}
    {step.status === 'skipped' ? <p>{t('tools.skipped_help')}</p> : null}
    {step.kind === 'connection' && step.status === 'succeeded' ? <p>{t('tools.connection_ok')}</p> : null}
    {step.output_text ? <pre className="model-test-output" data-user-content>{step.output_text}</pre> : null}
    {step.content_url ? <figure>
      <a href={step.content_url} target="_blank" rel="noreferrer" aria-label={t('tools.open_image')}><img src={step.content_url} alt={t('tools.image_alt')} width={step.image_width} height={step.image_height} /></a>
      <figcaption>{t('tools.image_dimensions', { width: step.image_width, height: step.image_height })} · <a href={step.content_url} target="_blank" rel="noreferrer">{t('tools.open_image')}</a></figcaption>
    </figure> : null}
    <LocalizedErrorMessage error={error} />
  </li>
}

export default function ModelTestPage() {
  const { testUuid } = useParams()
  const { t, formatDateTime } = useI18n()
  const navigate = useNavigate()
  const location = useLocation()
  const isNew = !testUuid
  const detail = useQuery({ queryKey: modelTestKeys.detail(testUuid), queryFn: () => getModelTest(testUuid), enabled: !isNew, retry: false })
  const test = detail.data
  const draft = isNew || test?.status === 'draft'
  const settings = useQuery({ queryKey: ['model-settings'], queryFn: getModelSettings, enabled: draft, retry: false })
  const providers = useQuery({ queryKey: ['providers'], queryFn: listProviders, enabled: draft, retry: false })
  const finished = test && ['succeeded', 'failed', 'interrupted'].includes(test.status)
  const steps = test?.steps || ['connection', 'text', 'image'].map((kind, index) => ({ kind, position: index + 1, status: 'pending' }))

  return <AppPageShell title={t('tools.model_test')}>
    <div className="model-tools-page">
      <header className="model-tools-heading">
        <Link className="model-tools-back" to={{ pathname: '/settings/tools', search: location.search }}><ArrowLeft size={15} aria-hidden="true" />{t('tools.back')}</Link>
        <h1>{t('tools.detail_title')}</h1><p>{t('tools.model_test_description')}</p>
      </header>
      <LocalizedErrorMessage error={detail.error || (draft ? settings.error || providers.error : null)} />
      {detail.isLoading || (draft && (settings.isLoading || providers.isLoading)) ? <p>{t('tools.loading')}</p> : null}
      {isNew || test ? <>
        {test ? <div className="model-test-summary" aria-live="polite"><TestStatus status={test.status} /><span>{formatDateTime(test.created_at, { dateStyle: 'medium', timeStyle: 'medium' })}</span>
          {['queued', 'running'].includes(test.status) ? <StopTestButton key={test.uuid} testUuid={test.uuid} /> : null}
          {finished ? <button type="button" className="button-secondary" onClick={() => navigate({ pathname: '/settings/tools/model-tests/new', search: location.search }, { state: { selection: repeatTestSelection(test) } })}>{t('tools.repeat')}</button> : null}
        </div> : null}
        {draft && settings.data && providers.data ? <ModelTestForm key={test?.uuid || location.key} test={test} selection={location.state?.selection} settings={settings.data} providers={providers.data} /> : null}
        {!draft ? <dl className="model-test-models model-test-frozen">{['text', 'image'].map((kind) => <div key={kind}><dt>{t(`tools.${kind}_model`)}</dt><dd data-no-i18n>{test[`${kind}_model`].provider_name} · {test[`${kind}_model`].model}</dd></div>)}</dl> : null}
        {test?.status === 'queued' ? <p role="status">{t('tools.queued_help')}</p> : null}
        {test?.status === 'interrupted' ? <p>{t('tools.interrupted_help')}</p> : null}
        <ol className="model-test-steps">{steps.map((step) => <StepResult key={step.kind} step={step} />)}</ol>
      </> : null}
    </div>
  </AppPageShell>
}
