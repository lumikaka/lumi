package textmcp

// Specs are built per server. Adapt their help before modelVisibleSpec appends
// the conditional field requirements shared by all hosts.
func genericAuthoringSpec(spec Spec) Spec {
	switch spec.Name {
	case "render_prompt":
		spec.Description = "Render a catalog template with project context, target UUID, expected_revision and write instructions. Execute text tasks using the returned prompt. Fragments also return fragment_prompt. Supply missing inputs in variables or narrow context_chapter_uuids/context_setting_uuids if needed. For page artwork use generate_page_image, which assembles the prompt itself. For a setting board, the user can use the returned generation_prompt to create artwork. Rendering does not call a provider."
	case "image_target":
		spec.Description = "Read the target canvas, composition guidance and expected_revision. For page_image, omit setting_uuids to inventory candidates, then select up to 12 settings needed by this page in reference order. For missing_images, import existing setting artwork or give the user a web_url to create it in Lumi. For review_required, inspect each selected image with read_image against the originals and user requirements. Bind setting_reviews to the current variant_uuid, revision and style_digest. With review_source_page_uuid, reuse matching previous_review only after confirming its reference basis. Use [] only for a page that needs no references. Then call generate_page_image with the selection, reviews, expected_revision and an idempotency_key."
	case "prepare_page_image":
		spec.Description = "Optional preview of the assembled page prompt and reference collage. Use the same explicit setting_uuids and current passed setting_reviews as generate_page_image. When reference_mode is collage, reference_image_path is the preview and reference_image_sha256 identifies its contents. To generate the page, call generate_page_image directly. Lumi assembles the references and prompt and attaches the result automatically."
	case "read_image":
		spec.Description = "View a project image by asset_uuid. Inspect selected setting references before generate_page_image. For the generated page, read the adopted image and compare it with the saved storyboard and user text before pages record_image_review. Get the current asset UUID from settings get or pages get."
	case "import_image":
		spec.Description = "Import an existing local PNG, JPEG or WebP, up to 32 MiB and 40M pixels. For a setting_image, get expected_revision from image_target. A manual page_image import requires the exact generation_prompt and unchanged signed preparation from prepare_page_image. Pages produced by generate_page_image are already attached. fit_mode defaults to keep and supports centered crop or pad."
	case "setting_boards":
		spec.Description = "List or get saved setting boards, or import an existing local board as one resumable batch. If artwork is missing, use web_url to let the user create or upload it in Lumi. Import preserves original pixels and records source text, style and prompt. Use premise.revision from settings list. Get returns the source, setting_image.asset for read_image, board revision and latest committed breakdown. Replay an import with its original key and inputs even if the file has expired."
	}
	if spec.Name == "setting_boards" || spec.Name == "import_image" {
		path := spec.Schema["properties"].(map[string]any)["path"].(map[string]any)
		path["description"] = "Absolute path to the existing local image file to import."
	}
	return spec
}
