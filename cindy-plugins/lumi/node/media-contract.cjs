'use strict';

// Adapt Go's path imports to a Host-authorized single attachment. Business
// fields and conditional requirements remain derived from textmcp.Specs().
function receiveMediaSchema(snapshot) {
  const byName = Object.fromEntries(snapshot.map(spec => [spec.Name, spec.Schema]));
  const image = structuredClone(byName.import_image);
  delete image.properties.path;
  image.required = image.required.filter(key => key !== 'path');
  const board = structuredClone(byName.setting_boards);
  const fields = Object.keys(board.properties).filter(key => !['action', 'path', 'board_uuid', 'before', 'after', 'limit'].includes(key));
  const boardProperties = Object.fromEntries(fields.map(key => [key, board.properties[key]]));
  const required = board.allOf.find(rule => rule.if.properties.action.const === 'import').then.required.filter(key => key !== 'path');
  return {
    type: 'object', additionalProperties: false,
    properties: { ...image.properties, ...boardProperties, slot: { type: 'string', enum: ['page_image', 'setting_image', 'setting_board'] } },
    required: ['slot', 'expected_revision'],
    allOf: [
      { if: { properties: { slot: { enum: ['page_image', 'setting_image'] } } }, then: image },
      { if: { properties: { slot: { const: 'setting_board' } } }, then: {
        type: 'object', additionalProperties: false,
        properties: { ...boardProperties, slot: { const: 'setting_board' } }, required,
      } },
    ],
  };
}

module.exports = { receiveMediaSchema };
