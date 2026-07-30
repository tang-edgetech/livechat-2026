const { loadConfig, readOffset, writeOffset } = require('./config');

async function pollOnce(botToken, chatId, offset) {
  const url = `https://api.telegram.org/bot${botToken}/getUpdates?timeout=25&offset=${offset}`;
  const res = await fetch(url, { signal: AbortSignal.timeout(35000) });
  const data = await res.json();
  if (!data.ok) throw new Error(JSON.stringify(data));

  let nextOffset = offset;
  for (const update of data.result) {
    nextOffset = update.update_id + 1;
    const msg = update.message;
    if (!msg || !msg.text) continue;
    if (String(msg.chat.id) !== String(chatId)) continue;
    const from = msg.from.username || msg.from.first_name || 'unknown';
    const text = msg.text.replace(/\n/g, ' ').trim();
    console.log(`TG_MSG|${from}|${text}`);
  }
  return nextOffset;
}

async function main() {
  const { botToken, chatId } = loadConfig();
  let offset = readOffset();
  console.log(`TG_LISTENING|chat ${chatId}`);
  while (true) {
    try {
      offset = await pollOnce(botToken, chatId, offset);
      writeOffset(offset);
    } catch (err) {
      console.error('poll error:', err.message);
      await new Promise(r => setTimeout(r, 3000));
    }
  }
}

main();
