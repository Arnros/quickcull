import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const ROOT_DIR = path.join(__dirname, '..');
const UI_SRC = path.join(ROOT_DIR, 'ui', 'src');

const EXCLUSIONS = [
  'i18n.t', 'appState', 'Icon', 'Logo', 'slot', 'svelte:', 'http', 'https',
  'px', '%', 'var(', 'aria-', 'lang=', 'style=', 'quickcull', 'quick', 'cull',
  'evt p:', 's:', 'c:', 'poll:', 'flush:', 'avg:', 'last:', 'Tab'
];

function isHardcoded(text) {
  const trimmed = text.trim();
  if (trimmed.length < 2) return false;
  if (/^[0-9\W]+$/.test(trimmed)) return false; 
  if (EXCLUSIONS.some(ex => trimmed.includes(ex))) return false;
  return true;
}

function checkFile(filePath) {
  const content = fs.readFileSync(filePath, 'utf8');
  const lines = content.split(/\r?\n/);
  const errors = [];

  lines.forEach((line, index) => {
    let cleanLine = line.replace(/\{[^{}]*\}/g, ' ');
    cleanLine = cleanLine.replace(/<!--.*-->/g, ' ');

    const tagMatches = cleanLine.match(/>([^<{]+)</g);
    if (tagMatches) {
      tagMatches.forEach(m => {
        const text = m.slice(1, -1);
        if (isHardcoded(text)) {
          errors.push({ line: index + 1, text: text.trim(), type: 'Tag Content' });
        }
      });
    }

    const attrMatches = cleanLine.match(/(title|placeholder|label)="([^"{]+)"/g);
    if (attrMatches) {
      attrMatches.forEach(m => {
        const parts = m.split('="');
        const value = parts[1].slice(0, -1);
        if (isHardcoded(value)) {
          errors.push({ line: index + 1, text: value, type: 'Attribute' });
        }
      });
    }
  });

  return errors;
}

function walkDir(dir, callback) {
  fs.readdirSync(dir).forEach(f => {
    const dirPath = path.join(dir, f);
    if (fs.statSync(dirPath).isDirectory()) walkDir(dirPath, callback);
    else if (f.endsWith('.svelte')) callback(dirPath);
  });
}

console.log('🚀 Starting Smart UI i18n Lint...');
let totalErrors = 0;

walkDir(UI_SRC, (filePath) => {
  const errors = checkFile(filePath);
  if (errors.length > 0) {
    console.log(`\n❌ ${path.relative(ROOT_DIR, filePath)}:`);
    errors.forEach(err => {
      console.log(`   [L${err.line}] (${err.type}): "${err.text}"`);
      totalErrors++;
    });
  }
});

if (totalErrors > 0) {
  console.log(`\n💥 Total: ${totalErrors} hardcoded strings found.`);
  process.exit(1);
} else {
  console.log('\n✅ UI i18n check passed!');
}
