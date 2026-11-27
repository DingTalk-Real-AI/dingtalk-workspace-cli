#!/usr/bin/env node
'use strict';

const fs = require('fs');
const path = require('path');
const {spawnSync} = require('child_process');

// checksums.txt is only a candidate inventory here. The sealed verifier below
// must validate it against that version's exact asset contract and verify bytes.
function verifySealedReleaseAssets(sourceRoot, version, dist, assetNames) {
  if (process.env.ACTIONS_ID_TOKEN_REQUEST_URL) throw new Error('asset verification must not hold publication credentials');
  sourceRoot = fs.realpathSync(sourceRoot);
  dist = fs.realpathSync(dist);
  const inventory = fs.readFileSync(path.join(dist, 'checksums.txt'), 'utf8').trim().split(/\r?\n/).map(line => {
    const match = /^([a-fA-F0-9]{64})\s+(dws-[A-Za-z0-9._-]+\.(?:tar\.gz|zip))$/.exec(line);
    if (!match) throw new Error('invalid sealed release checksum entry');
    return match[2];
  });
  const expected = ['checksums.txt', ...inventory];
  if (new Set(expected).size !== expected.length) throw new Error('duplicate sealed release checksum entry');
  function requireExactSet(names, label) {
    if (!Array.isArray(names) || names.length !== expected.length || new Set(names).size !== expected.length ||
        names.some(name => !expected.includes(name))) {
      throw new Error(`${label} does not contain the exact sealed release asset set`);
    }
  }
  requireExactSet(assetNames, 'GitHub Release');
  requireExactSet(fs.readdirSync(dist), 'download directory');
  for (const name of expected) {
    if (!fs.lstatSync(path.join(dist, name)).isFile()) throw new Error(`release asset is not a regular file: ${name}`);
  }
  const verifier = path.join(sourceRoot, 'scripts', 'release', 'verify-release-artifacts.sh');
  const result = spawnSync('sh', [verifier, version], {
    cwd: sourceRoot, stdio: 'inherit', env: {...process.env, DWS_PACKAGE_DIST_DIR: dist},
  });
  if (result.error) throw result.error;
  if (result.status !== 0) throw new Error(`sealed release artifact verification failed (${result.status ?? result.signal})`);
}

module.exports = {verifySealedReleaseAssets};
if (require.main === module) {
  try {
    verifySealedReleaseAssets(process.argv[2], process.argv[3], process.argv[4], JSON.parse(process.env.DWS_RELEASE_ASSET_NAMES));
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
