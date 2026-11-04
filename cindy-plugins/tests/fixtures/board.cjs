'use strict';
const { deflateSync } = require('node:zlib');

function boardPNG() {
  const width = 300, height = 200, rows = Buffer.alloc((width * 3 + 1) * height, 255);
  for (let y = 0; y < height; y++) {
    rows[y * (width * 3 + 1)] = 0;
    for (let x = 0; x < width; x++) {
      const left = x >= 30 && x < 120 && y >= 40 && y < 160;
      const right = x >= 180 && x < 270 && y >= 40 && y < 160;
      if (left || right) rows.set(left ? [210, 30, 30] : [30, 70, 210], y * (width * 3 + 1) + 1 + x * 3);
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
module.exports = { boardPNG };
