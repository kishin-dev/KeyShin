// Dashboard: confirms the admin is signed in, then loads the overview.
//
// Note: the session check here only decides what the browser shows. The real
// protection is on the server: every admin API endpoint checks the session
// itself, so the data stays private even if someone opens this HTML directly.
(function () {
  const userEl = document.getElementById('session-user');
  const logoutEl = document.getElementById('logout');
  const statsErrorEl = document.getElementById('stats-error');
  const emptyEl = document.getElementById('empty-products');

  function toLogin() {
    location.replace('/');
  }

  async function getJSON(url) {
    const res = await fetch(url, { credentials: 'same-origin' });
    if (res.status === 401) {
      toLogin();
      throw new Error('signed out');
    }
    const data = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(data.error || `error ${res.status}`);
    return data;
  }

  async function loadStats() {
    try {
      const stats = await getJSON('/api/stats');
      for (const [name, value] of Object.entries(stats)) {
        const el = document.querySelector(`[data-stat="${name}"]`);
        if (el) el.textContent = Number(value).toLocaleString();
      }
      emptyEl.classList.toggle('hidden', stats.products > 0);
    } catch (err) {
      if (err.message === 'signed out') return;
      statsErrorEl.textContent = `Couldn't load the totals: ${err.message}. Refresh the page to try again.`;
      statsErrorEl.classList.remove('hidden');
    }
  }

  async function start() {
    try {
      const admin = await getJSON('/api/session');
      userEl.textContent = admin.username;
      document.body.classList.remove('checking-session');
      loadStats();
    } catch {
      toLogin();
    }
  }

  logoutEl.addEventListener('click', async () => {
    logoutEl.disabled = true;
    logoutEl.textContent = 'Logging out…';
    try {
      await fetch('/api/logout', { method: 'POST', credentials: 'same-origin' });
    } finally {
      toLogin();
    }
  });

  start();
})();
