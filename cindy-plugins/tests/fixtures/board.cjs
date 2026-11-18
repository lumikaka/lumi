'use strict';
const { deflateSync, inflateSync } = require('node:zlib');

function boardPNG({ width = 300, height = 200, color } = {}) {
  const rows = Buffer.alloc((width * 3 + 1) * height, 255);
  for (let y = 0; y < height; y++) {
    rows[y * (width * 3 + 1)] = 0;
    for (let x = 0; x < width; x++) {
      const left = x >= 30 && x < 120 && y >= 40 && y < 160;
      const right = x >= 180 && x < 270 && y >= 40 && y < 160;
      if (color || left || right) rows.set(color || (left ? [210, 30, 30] : [30, 70, 210]), y * (width * 3 + 1) + 1 + x * 3);
    }
  }
  const chunk = (name, data) => {
    const body = Buffer.concat([Buffer.from(name), data]); let crc = 0xffffffff;
    for (const byte of body) { crc ^= byte; for (let bit = 0; bit < 8; bit++) crc = (crc >>> 1) ^ (crc & 1 ? 0xedb88320 : 0); }
    const prefix = Buffer.alloc(4), suffix = Buffer.alloc(4);
    prefix.writeUInt32BE(data.length); suffix.writeUInt32BE((crc ^ 0xffffffff) >>> 0);
    return Buffer.concat([prefix, body, suffix]);
  };
  const header = Buffer.alloc(13); header.writeUInt32BE(width); header.writeUInt32BE(height, 4); header[8] = 8; header[9] = 2;
  return Buffer.concat([Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]), chunk('IHDR', header), chunk('IDAT', deflateSync(rows)), chunk('IEND', Buffer.alloc(0))]);
}
// Decode the small 8-bit RGB/RGBA fixtures and Go PNG output for exact pixel
// assertions. This is test-only; production image decoding remains in Go.
function pngPixels(bytes) {
  const width = bytes.readUInt32BE(16), height = bytes.readUInt32BE(20);
  const channels = { 2: 3, 6: 4 }[bytes[25]];
  if (bytes[24] !== 8 || !channels || bytes[28] !== 0) throw new Error('Unsupported test PNG');
  const compressed = [];
  for (let at = 8; at < bytes.length;) {
    const length = bytes.readUInt32BE(at), name = bytes.toString('ascii', at + 4, at + 8);
    if (name === 'IDAT') compressed.push(bytes.subarray(at + 8, at + 8 + length));
    at += length + 12;
  }
  const raw = inflateSync(Buffer.concat(compressed)), stride = width * channels, rows = Buffer.alloc(stride * height);
  const paeth = (a, b, c) => {
    const p = a + b - c, da = Math.abs(p - a), db = Math.abs(p - b), dc = Math.abs(p - c);
    return da <= db && da <= dc ? a : db <= dc ? b : c;
  };
  for (let y = 0; y < height; y++) {
    const filter = raw[y * (stride + 1)];
    if (filter > 4) throw new Error('Invalid test PNG filter');
    for (let x = 0; x < stride; x++) {
      const offset = y * stride + x, a = x >= channels ? rows[offset - channels] : 0;
      const b = y ? rows[offset - stride] : 0, c = y && x >= channels ? rows[offset - stride - channels] : 0;
      rows[offset] = raw[y * (stride + 1) + 1 + x] + [0, a, b, Math.floor((a + b) / 2), paeth(a, b, c)][filter];
    }
  }
  return { width, height, at(x, y) { const offset = (y * width + x) * channels; return [...rows.subarray(offset, offset + 3), channels === 4 ? rows[offset + 3] : 255]; } };
}
module.exports = { boardPNG, pngPixels };
