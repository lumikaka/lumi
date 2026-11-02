import fs from 'node:fs';
import path from 'node:path';
import { randomUUID } from 'node:crypto';

// Replacing the directory entry preserves the old inode for a running native
// process. Never truncate its executable through cpSync/copyFileSync in place.
export function copyNative(source, target) {
  const stat = fs.lstatSync(source);
  if (!stat.isFile()) throw new Error('Native source must be a regular file');
  const parent = path.dirname(target);
  fs.mkdirSync(parent, { recursive: true });
  if (!fs.lstatSync(parent).isDirectory()) throw new Error('Native destination must be a real directory');
  try {
    if (!fs.lstatSync(target).isFile()) throw new Error('Native destination must be a regular file');
  } catch (error) { if (error.code !== 'ENOENT') throw error; }
  const temporary = path.join(parent, '.' + path.basename(target) + '.' + randomUUID() + '.tmp');
  try {
    fs.copyFileSync(source, temporary, fs.constants.COPYFILE_EXCL);
    fs.chmodSync(temporary, stat.mode & 0o777);
    fs.renameSync(temporary, target);
  } finally {
    fs.rmSync(temporary, { force: true });
  }
}
