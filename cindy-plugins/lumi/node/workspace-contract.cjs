'use strict';
const launchTools = ['review_book', 'review_chapters', 'review_chapter', 'review_page', 'review_settings', 'review_candidates'];
const pageTools = ['read_app_resource', 'write_app_resource', 'read_app_image', 'read_app_image_detail', 'watch_app_changes'];
const resourceURI = 'ui://lumi/workspace/v1';

function workspaceSchemas(snapshot) {
  const specs = Object.fromEntries(snapshot.map(spec => [spec.Name, spec.Schema]));
  const uuid = specs.read_app_image.properties.asset_uuid;
  return Object.fromEntries([...launchTools, ...pageTools].map(name => {
    if (!specs[name]) throw new Error(`Missing workspace contract: ${name}`);
    const schema = structuredClone(specs[name]);
    if (pageTools.includes(name)) {
      schema.properties.expected_project_uuid = { ...uuid, description: 'Identity constraint from this card launch; never selects a project or task.' };
      schema.required = [...(schema.required || []), 'expected_project_uuid'];
    }
    return [name, schema];
  }));
}
module.exports = { launchTools, pageTools, resourceURI, workspaceSchemas };
