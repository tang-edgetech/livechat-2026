const fs = require('fs');
const path = require('path');

const CONFIG_PATH = path.join(__dirname, '..', '..', '.claude-telegram', 'config.json');
const OFFSET_PATH = path.join(__dirname, '..', '..', '.claude-telegram', 'offset.txt');

function loadConfig() {
  return JSON.parse(fs.readFileSync(CONFIG_PATH, 'utf8'));
}

function readOffset() {
  try {
    return parseInt(fs.readFileSync(OFFSET_PATH, 'utf8').trim(), 10) || 0;
  } catch {
    return 0;
  }
}

function writeOffset(offset) {
  fs.writeFileSync(OFFSET_PATH, String(offset));
}

module.exports = { loadConfig, readOffset, writeOffset };
