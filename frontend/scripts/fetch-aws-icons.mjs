#!/usr/bin/env node
// AWS 公式アイコンパッケージ (zip) から未改変の SVG を抽出し、
// public/assets/aws-icons/<service>.svg に配置するセットアップスクリプト。
//
// 使い方: node scripts/fetch-aws-icons.mjs <path-to-aws-icons.zip>
//
// AWS 公式サイトの Asset Package zip 構成 (バージョンにより変わりうる) に対応するため、
// ファイル名一致で検索する (ディレクトリ階層には依存しない)。

import { execFileSync } from 'node:child_process';
import { mkdtempSync, rmSync, readdirSync, statSync, copyFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, dirname, basename } from 'node:path';
import { fileURLToPath } from 'node:url';

const SCRIPT_DIR = dirname(fileURLToPath(import.meta.url));
const OUTPUT_DIR = join(SCRIPT_DIR, '..', 'public', 'assets', 'aws-icons');

// サービスキー -> AWS 公式パッケージ内のファイル名
// natgw は Architecture Icons (32px) に該当アイコンが存在しないため、
// Resource Icons (48px) の NAT Gateway アイコンを使用する。
const ICON_FILENAMES = {
  ec2: 'Arch_Amazon-EC2_32.svg',
  s3: 'Arch_Amazon-Simple-Storage-Service_32.svg',
  rds: 'Arch_Amazon-RDS_32.svg',
  ecs: 'Arch_Amazon-Elastic-Container-Service_32.svg',
  ecr: 'Arch_Amazon-Elastic-Container-Registry_32.svg',
  lambda: 'Arch_AWS-Lambda_32.svg',
  dynamo: 'Arch_Amazon-DynamoDB_32.svg',
  cache: 'Arch_Amazon-ElastiCache_32.svg',
  elb: 'Arch_Elastic-Load-Balancing_32.svg',
  iam: 'Arch_AWS-Identity-and-Access-Management_32.svg',
  cloudfront: 'Arch_Amazon-CloudFront_32.svg',
  sqs: 'Arch_Amazon-Simple-Queue-Service_32.svg',
  kinesis: 'Arch_Amazon-Kinesis_32.svg',
  waf: 'Arch_AWS-WAF_32.svg',
  ssm: 'Arch_AWS-Systems-Manager_32.svg',
  secrets: 'Arch_AWS-Secrets-Manager_32.svg',
  apigw: 'Arch_Amazon-API-Gateway_32.svg',
  natgw: 'Res_Amazon-VPC_NAT-Gateway_48.svg',
};

function extractZip(zipPath, destDir) {
  execFileSync('unzip', ['-q', '-o', zipPath, '-d', destDir]);
}

function extractNestedZips(dir) {
  for (const entry of walk(dir)) {
    if (entry.toLowerCase().endsWith('.zip')) {
      extractZip(entry, dirname(entry));
    }
  }
}

function* walk(dir) {
  for (const name of readdirSync(dir)) {
    const full = join(dir, name);
    const st = statSync(full);
    if (st.isDirectory()) {
      yield* walk(full);
    } else {
      yield full;
    }
  }
}

function buildFilenameIndex(dir) {
  const index = new Map();
  for (const full of walk(dir)) {
    const name = basename(full);
    if (!index.has(name)) {
      index.set(name, full);
    }
  }
  return index;
}

function main() {
  const zipPath = process.argv[2];
  if (!zipPath) {
    console.error('使い方: node scripts/fetch-aws-icons.mjs <path-to-aws-icons.zip>');
    process.exit(1);
  }

  const tmpDir = mkdtempSync(join(tmpdir(), 'aws-icons-'));
  try {
    extractZip(zipPath, tmpDir);
    extractNestedZips(tmpDir);

    const index = buildFilenameIndex(tmpDir);
    const missing = [];

    for (const [key, filename] of Object.entries(ICON_FILENAMES)) {
      const src = index.get(filename);
      if (!src) {
        missing.push(`${key} (${filename})`);
        continue;
      }
      copyFileSync(src, join(OUTPUT_DIR, `${key}.svg`));
      console.log(`ok: ${key}.svg <- ${filename}`);
    }

    if (missing.length > 0) {
      console.error(`見つからなかったアイコン: ${missing.join(', ')}`);
      process.exit(1);
    }
  } finally {
    rmSync(tmpDir, { recursive: true, force: true });
  }
}

main();
