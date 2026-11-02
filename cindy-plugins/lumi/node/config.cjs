'use strict';
const path = require('node:path');
const os = require('node:os');

// Build-owned configuration. Never populated from Agent or taskView input.
function runtimeOptions(config, author, home = os.homedir()) {
  const development = config?.channel === 'development';
  if ((!development && config?.channel !== 'production')
    || config.environment !== (development ? 'development' : 'production')
    || config.binary !== (development ? 'lumi_dev' : 'lumi')
    || (config.data_dir !== null && (typeof config.data_dir !== 'string'
      || !path.isAbsolute(config.data_dir) || config.data_dir.includes('\0') || !development))) {
    throw new Error('Invalid build-owned runtime configuration');
  }
  return { executable: path.join(author, 'bin', config.binary), environment: config.environment,
    dataDir: config.data_dir === null ? path.join(home, development ? '.lumi_dev' : '.lumi') : path.normalize(config.data_dir) };
}
module.exports = { runtimeOptions };
