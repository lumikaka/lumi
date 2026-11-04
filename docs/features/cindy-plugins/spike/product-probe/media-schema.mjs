// Add the test selector at each constraint on the same object, without
// relaxing nested business objects or copying their evolving field lists.
export function fixtureMediaTool(original) {
  const media = structuredClone(original);
  const fixture = { type: 'string', enum: ['a', 'b'] };
  function extend(schema) {
    if (!schema || typeof schema !== 'object') return;
    if (schema.properties && schema.additionalProperties === false) schema.properties.fixture = fixture;
    for (const key of ['allOf', 'anyOf', 'oneOf']) for (const item of schema[key] || []) extend(item);
    extend(schema.then); extend(schema.else);
  }
  extend(media.parameters);
  media.parameters.required.push('fixture');
  return media;
}
