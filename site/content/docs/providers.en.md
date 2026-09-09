---
title: Configure a model provider
description: Connect Alibaba Cloud Model Studio or Cloudflare AI Gateway, then store and verify the credentials safely.
translationKey: providers
slug: providers
docs_group: start
weight: 20
keywords: [model, provider, Alibaba Cloud, Cloudflare, API key, token, connection]
---

Lumi does not include hosted model credits. On first launch, choose a provider and enter credentials created in that provider’s account. One current provider supplies the fallback defaults; model settings can also select other ready providers.

## Choose a provider

### Alibaba Cloud Model Studio

Prepare:

- An API key
- A workspace ID
- The region that hosts that workspace

Lumi uses built-in text and image model settings. Enter the details and choose “Connect and start using” or “Save and verify.” Use the provider for projects only after the connection check succeeds.

### Cloudflare AI Gateway

Prepare:

- A Cloudflare Account ID
- A Cloudflare API token

Lumi derives the Responses API endpoint from the Account ID. Text and image models each default to `openai/gpt-5.6-terra`. You can independently choose Terra or `openai/gpt-5.6-sol` from the dropdowns. The token needs Account → Workers AI → Read permission. If verification is requested after an upgrade, save and verify the connection again.

## Save and verify

1. Choose a provider on the first-launch screen.
2. Enter the requested IDs, region, and secret.
3. Select “Connect and start using.” After onboarding, reopen these fields from Settings → Providers.
4. Wait for “Connection verified.” If the check fails, keep the visible error and verify the credentials, workspace/account, and network one at a time.

{{< callout type="note" title="How secrets are stored" >}}
Lumi encrypts the API key or token on this device and does not reveal the complete value again. Saving an empty secret field keeps the existing secret. “Reset key” removes the locally stored credential.
{{< /callout >}}

## Change the current provider

You may save more than one provider configuration, but only one is current. Open Settings → Providers and choose “Make current provider” on a verified configuration. New tasks without global or project model overrides use the new current setting; tasks already enqueued keep the model settings frozen when they were created.

## Configure default project models

In Settings → Providers, the “Default project models” section configures text, images, chat, story and comic scripts, and reference selection. Changes save immediately; choose “Restore inheritance” to clear a selection. Supported image models also offer thinking and prompt rewriting switches.

New and existing projects continuously inherit these defaults, with project settings taking priority. Text scenarios use: project scenario override → project text override → global scenario default → global text default → active provider default. Images use the project image override, then the global image default, then the active provider default. An explicit task model takes priority over these settings.

For example, if the global story model is A and a project's text default is B, that project generates stories with B. Clearing the project override restores the latest global setting. Changing global defaults does not change existing tasks.

Each field shows the inherited and effective model. Unavailable saved selections remain visible with a warning and fall back to an available inherited model. If none is available, fix the provider connection or model settings before generating.

## Common connection issues

- **Missing credential**: make sure the Account ID, workspace ID, and secret are in the correct fields.
- **Connection check failed**: verify that the credential is valid, the region is correct, and your network can reach the provider.
- **Secret unavailable**: the root key in operating-system secure storage may be missing or changed; enter and save the credential again.
- **Usage charges**: your provider bills usage according to its account and model terms. Lumi only displays call and usage information collected locally.
