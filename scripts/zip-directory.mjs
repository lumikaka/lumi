// Small ZIP32 writer for our plugin's regular files. Node >= 22.2 supplies CRC32;
// no package manager, external zip program, or platform-specific hidden-file rules.
import { lstatSync, readdirSync, readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { crc32, deflateRawSync } from 'node:zlib';

export function zipDirectory(source, archive, rootName) {
  const entries = [];
  function visit(directory, prefix) {
    for (const name of readdirSync(directory).sort()) {
      const path = join(directory, name);
      const stat = lstatSync(path);
      const member = `${prefix}/${name}`;
      if (stat.isDirectory()) visit(path, member);
      else if (stat.isFile()) entries.push({ path, member });
      else throw new Error(`Plugin package contains a link or special file: ${path}`);
    }
  }
  visit(source, rootName);
  if (entries.length > 65535) throw new Error('Plugin exceeds ZIP32 entry limit');
  const local = [], central = [];
  let offset = 0;
  for (const entry of entries) {
    const name = Buffer.from(entry.member, 'utf8');
    const data = readFileSync(entry.path);
    const compressed = deflateRawSync(data);
    const header = Buffer.alloc(30);
    header.writeUInt32LE(0x04034b50, 0);
    header.writeUInt16LE(20, 4);
    header.writeUInt16LE(0x800, 6); // UTF-8 names
    header.writeUInt16LE(8, 8); // deflate
    header.writeUInt16LE(33, 12); // 1980-01-01; deterministic packaging
    header.writeUInt32LE(crc32(data), 14);
    header.writeUInt32LE(compressed.length, 18);
    header.writeUInt32LE(data.length, 22);
    header.writeUInt16LE(name.length, 26);
    const record = Buffer.alloc(46);
    record.writeUInt32LE(0x02014b50, 0);
    record.writeUInt16LE(0x0314, 4); // Unix, ZIP 2.0
    header.copy(record, 6, 4, 30);
    record.writeUInt32LE((0o100644 << 16) >>> 0, 38);
    record.writeUInt32LE(offset, 42);
    local.push(header, name, compressed);
    central.push(record, name);
    offset += header.length + name.length + compressed.length;
  }
  const directory = Buffer.concat(central);
  const end = Buffer.alloc(22);
  end.writeUInt32LE(0x06054b50, 0);
  end.writeUInt16LE(entries.length, 8);
  end.writeUInt16LE(entries.length, 10);
  end.writeUInt32LE(directory.length, 12);
  end.writeUInt32LE(offset, 16);
  writeFileSync(archive, Buffer.concat([...local, directory, end]));
}
