const labelEl = document.getElementById('label');
const portEl = document.getElementById('port');
const savedEl = document.getElementById('saved');

chrome.storage.local.get(['label', 'port']).then(({ label = '', port = '' }) => {
  labelEl.value = label;
  portEl.value = port;
});

document.getElementById('save').addEventListener('click', async () => {
  const port = portEl.value.trim();
  await chrome.storage.local.set({ label: labelEl.value.trim(), port: /^\d+$/.test(port) ? port : '' });
  savedEl.textContent = 'Saved';
  setTimeout(() => { savedEl.textContent = ''; }, 1500);
});
