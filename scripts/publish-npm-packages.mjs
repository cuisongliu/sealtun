#!/usr/bin/env node

import { spawnSync } from 'node:child_process';
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { readdir, stat } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const rootDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

function parseArgs(argv) {
  const args = {
    outDir: process.env.NPM_PACKAGES_DIR || path.join(rootDir, 'packages'),
    distTag: process.env.NPM_DIST_TAG || 'latest',
    skipExisting: process.env.NPM_SKIP_EXISTING === '1',
    retries: Number(process.env.NPM_PUBLISH_RETRIES || 3),
    report: process.env.NPM_PUBLISH_REPORT || '',
    npmArgs: []
  };

  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    if (arg === '--') {
      args.npmArgs = argv.slice(index + 1);
      break;
    }

    const readValue = () => {
      const value = argv[index + 1];
      if (!value || value.startsWith('--')) {
        throw new Error(`${arg} requires a value`);
      }
      index += 1;
      return value;
    };

    switch (arg) {
      case '--out-dir':
        args.outDir = readValue();
        break;
      case '--dist-tag':
        args.distTag = readValue();
        break;
      case '--skip-existing':
        args.skipExisting = readValue() === '1';
        break;
      case '--retries':
        args.retries = Number(readValue());
        break;
      case '--report':
        args.report = readValue();
        break;
      case '--help':
        printHelp();
        process.exit(0);
        break;
      default:
        throw new Error(`Unknown argument: ${arg}`);
    }
  }

  if (!Number.isInteger(args.retries) || args.retries < 0 || args.retries > 10) {
    throw new Error(`--retries must be an integer between 0 and 10, got ${args.retries}`);
  }

  args.outDir = path.resolve(rootDir, args.outDir);
  if (args.report) {
    args.report = path.resolve(rootDir, args.report);
  }
  return args;
}

function printHelp() {
  console.log(`Usage: node scripts/publish-npm-packages.mjs [options] -- [npm publish args]

Options:
  --out-dir <dir>         generated package directory, default packages
  --dist-tag <tag>        npm dist-tag, default latest
  --skip-existing <0|1>   skip versions that already exist in npm
  --retries <n>           retry transient publish failures, default 3
  --report <path>         write a JSON publish report
`);
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function run(command, args, options = {}) {
  const result = spawnSync(command, args, {
    cwd: options.cwd,
    env: options.env || process.env,
    encoding: 'utf8'
  });
  return {
    status: result.status ?? 1,
    stdout: result.stdout || '',
    stderr: result.stderr || '',
    error: result.error || null
  };
}

function readPackageManifest(packageDir) {
  return JSON.parse(readFileSync(path.join(packageDir, 'package.json'), 'utf8'));
}

async function listPackages(outDir) {
  const entries = await readdir(outDir);
  const packageDirs = [];
  for (const entry of entries.sort()) {
    const fullPath = path.join(outDir, entry);
    const entryStat = await stat(fullPath);
    if (!entryStat.isDirectory()) {
      continue;
    }
    if (existsSync(path.join(fullPath, 'package.json'))) {
      packageDirs.push(fullPath);
    }
  }
  return [...packageDirs, outDir];
}

function packageExists(name, version) {
  const result = run('npm', ['view', `${name}@${version}`, 'version', '--json']);
  return result.status === 0;
}

function isAlreadyPublishedOutput(output) {
  return /previously published version|cannot publish over the previously published versions|forbidden.*cannot modify pre-existing version/i.test(
    output
  );
}

function isRetryableOutput(output) {
  return /(EAI_AGAIN|ECONNRESET|ECONNREFUSED|ETIMEDOUT|socket hang up|502 Bad Gateway|503 Service Unavailable|504 Gateway Timeout|429 Too Many Requests|fetch failed|network timeout|unexpected EOF)/i.test(
    output
  );
}

function buildSummaryMarkdown(report) {
  const lines = [
    '## npm publish summary',
    '',
    `- dist-tag: \`${report.distTag}\``,
    `- retries: ${report.retries}`,
    `- packages: ${report.packages.length}`,
    `- published: ${report.counts.published}`,
    `- skipped_existing: ${report.counts.skipped_existing}`,
    `- failed: ${report.counts.failed}`,
    ''
  ];

  for (const pkg of report.packages) {
    lines.push(
      `- \`${pkg.name}@${pkg.version}\` -> ${pkg.status} (attempts: ${pkg.attempts}, dir: ${pkg.dir})`
    );
    if (pkg.error) {
      lines.push(`  - error: ${pkg.error}`);
    }
  }

  lines.push('');
  return `${lines.join('\n')}\n`;
}

async function publishPackage(pkg, args) {
  if (args.skipExisting && packageExists(pkg.name, pkg.version)) {
    return {
      ...pkg,
      status: 'skipped_existing',
      attempts: 0,
      error: ''
    };
  }

  for (let attempt = 1; attempt <= args.retries + 1; attempt += 1) {
    console.log(`Publishing ${pkg.name}@${pkg.version} from ${pkg.dir} (attempt ${attempt}/${args.retries + 1})`);
    const result = run('npm', ['publish', '--tag', args.distTag, ...args.npmArgs], { cwd: pkg.absDir });
    if (result.stdout) {
      process.stdout.write(result.stdout);
    }
    if (result.stderr) {
      process.stderr.write(result.stderr);
    }

    const combinedOutput = [result.stdout, result.stderr, result.error?.message || ''].join('\n');
    if (result.status === 0) {
      return {
        ...pkg,
        status: 'published',
        attempts: attempt,
        error: ''
      };
    }

    if (args.skipExisting && (isAlreadyPublishedOutput(combinedOutput) || packageExists(pkg.name, pkg.version))) {
      return {
        ...pkg,
        status: 'skipped_existing',
        attempts: attempt,
        error: 'version already exists in npm registry'
      };
    }

    if (attempt <= args.retries && isRetryableOutput(combinedOutput)) {
      const waitMs = attempt * 2000;
      console.log(`Transient npm publish failure for ${pkg.name}@${pkg.version}; retrying in ${waitMs}ms`);
      await sleep(waitMs);
      continue;
    }

    return {
      ...pkg,
      status: 'failed',
      attempts: attempt,
      error: combinedOutput.trim() || `npm publish exited with status ${result.status}`
    };
  }

  return {
    ...pkg,
    status: 'failed',
    attempts: args.retries + 1,
    error: 'publish loop exited unexpectedly'
  };
}

async function main() {
  const args = parseArgs(process.argv.slice(2));
  const packageDirs = await listPackages(args.outDir);
  const packages = packageDirs.map((dir) => {
    const manifest = readPackageManifest(dir);
    return {
      absDir: dir,
      dir: path.relative(rootDir, dir),
      name: manifest.name,
      version: manifest.version
    };
  });

  const report = {
    startedAt: new Date().toISOString(),
    distTag: args.distTag,
    retries: args.retries,
    npmArgs: args.npmArgs,
    packages: [],
    counts: {
      published: 0,
      skipped_existing: 0,
      failed: 0
    }
  };

  for (const pkg of packages) {
    const result = await publishPackage(pkg, args);
    report.packages.push(result);
    if (result.status === 'published') {
      report.counts.published += 1;
    } else if (result.status === 'skipped_existing') {
      report.counts.skipped_existing += 1;
    } else {
      report.counts.failed += 1;
    }
  }

  report.finishedAt = new Date().toISOString();

  const summary = buildSummaryMarkdown(report);
  process.stdout.write(summary);
  if (process.env.GITHUB_STEP_SUMMARY) {
    writeFileSync(process.env.GITHUB_STEP_SUMMARY, summary, { flag: 'a' });
  }
  if (args.report) {
    mkdirSync(path.dirname(args.report), { recursive: true });
    writeFileSync(args.report, `${JSON.stringify(report, null, 2)}\n`);
  }

  if (report.counts.failed > 0) {
    process.exit(1);
  }
}

main().catch((error) => {
  console.error(error.message);
  process.exit(1);
});
