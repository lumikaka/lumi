import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('./ProviderSettingsPage.jsx', import.meta.url), 'utf8')
const appSource = readFileSync(new URL('../App.jsx', import.meta.url), 'utf8')
const styles = readFileSync(new URL('../styles/settings.sass', import.meta.url), 'utf8')
const messages = readFileSync(new URL('../i18n/messages/settings.js', import.meta.url), 'utf8')

test('provider setup uses a selectable list and opens one configuration dialog', () => {
  assert.match(source, /className="provider-list"/)
  assert.match(source, /function ProviderListItem/)
  assert.match(source, /<LumiDialog[\s\S]*className="provider-dialog"[\s\S]*dismissDisabled={busy}/)
  assert.match(source, /selectedProvider && settingsQuery\.data/)
  assert.doesNotMatch(source, /className={`provider-card/)
  assert.match(styles, /\.provider-list-item--active[\s\S]*&:hover,/)
})

test('initial setup keeps the provider list and dialog on the dedicated setup URL', () => {
  const saveIndex = source.indexOf('await updateSiteSettings(next)')
  const checkIndex = source.indexOf('await checkProvider(provider.uuid)')
  const activateIndex = source.indexOf("if (onboarding) await updateSiteSettings({ 'ai_provider.active': provider.provider_type })")

  assert.ok(saveIndex >= 0)
  assert.ok(checkIndex > saveIndex)
  assert.ok(activateIndex > checkIndex)
  assert.match(appSource, /path="\/setup\/\*" element={<ProviderSettingsPage onboarding \/>}/)
  assert.match(appSource, /<Navigate replace to="\/setup\/" state={{ from }} \/>/)
  assert.match(source, /sortedItems\.map[\s\S]*<ProviderListItem/)
  assert.match(source, /<ProviderDialog[\s\S]*onboarding={onboarding}/)
  assert.match(source, /settings\.provider\.connect_start/)
  assert.match(messages, /'settings\.provider\.connect_start': \['连接并开始使用'/)
})

test('initial Cloudflare dialog omits model fields while regular settings retain them', () => {
  assert.match(source, /initialProviderForm\(provider, onboarding = false\)/)
  assert.match(source, /\.\.\.\(onboarding \? {} : {[\s\S]*default_model[\s\S]*default_image_model/)
  assert.match(source, /!onboarding \? <>[\s\S]*settings\.provider\.default_text_model[\s\S]*settings\.provider\.default_image_model[\s\S]*settings\.provider\.cloudflare_setup_defaults/)
  assert.match(source, /settings\.provider\.cloudflare_account_id/)
  assert.match(source, /settings\.provider\.cloudflare_api_token/)
})

test('the Cloudflare provider has a fixed gateway endpoint and no generic OpenAI URL field', () => {
  assert.match(source, /cloudflare_ai_gateway: 'ai_providers\.openai_compatible'/)
  assert.match(source, /settings\.provider\.cloudflare_account_id/)
  assert.match(source, /settings\.provider\.cloudflare_api_token/)
  assert.match(source, /api\.cloudflare\.com\/client\/v4\/accounts\/\$\{normalized\}\/ai\/v1/)
  assert.doesNotMatch(source, /https:\/\/api\.openai\.com\/v1/)
  assert.doesNotMatch(messages, /OpenAI 兼容接口/)
})
