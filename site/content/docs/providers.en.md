---
title: Configure a model provider
description: Connect Alibaba Cloud Model Studio or Cloudflare AI Gateway, then store and verify the credentials safely.
translationKey: providers
slug: providers
docs_group: start
weight: 20
keywords: [model, provider, Alibaba Cloud, Cloudflare, API key, token, connection]
---

Lumi does not include hosted model credits. On first launch, choose a provider and enter credentials created in that provider’s account. Product workflows use one current provider at a time.

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

Lumi derives the endpoint from the Account ID. You do not need to choose text or image models during initial setup. The token must have permission to call the relevant Gateway.

## Save and verify

1. Choose a provider on the first-launch screen.
2. Enter the requested IDs, region, and secret.
3. Select “Connect and start using.” After onboarding, reopen these fields from Settings → Providers.
4. Wait for “Connection verified.” If the check fails, keep the visible error and verify the credentials, workspace/account, and network one at a time.

{{< callout type="note" title="How secrets are stored" >}}
Lumi encrypts the API key or token on this device and does not reveal the complete value again. Saving an empty secret field keeps the existing secret. “Reset key” removes the locally stored credential.
{{< /callout >}}

## Change the current provider

You may save more than one provider configuration, but only one is current for product workflows. Open Settings → Providers and choose “Make current provider” on a verified configuration. New tasks use the new current setting; tasks already enqueued keep the model settings frozen when they were created.

## Common connection issues

- **Missing credential**: make sure the Account ID, workspace ID, and secret are in the correct fields.
- **Connection check failed**: verify that the credential is valid, the region is correct, and your network can reach the provider.
- **Secret unavailable**: the root key in operating-system secure storage may be missing or changed; enter and save the credential again.
- **Usage charges**: your provider bills usage according to its account and model terms. Lumi only displays call and usage information collected locally.
