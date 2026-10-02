// Overview page: the totals across all products.
(function () {
  const { api, ready } = window.KS;
  const statsErrorEl = document.getElementById('stats-error');

  async function loadStats() {
    try {
      const stats = await api('/api/stats');
      for (const [name, value] of Object.entries(stats)) {
        const el = document.querySelector(`[data-stat="${name}"]`);
        if (el) el.textContent = Number(value).toLocaleString();
      }
      document.getElementById('empty-products').hidden = stats.products > 0;
      document.getElementById('quick-actions').hidden = stats.products === 0;
    } catch (err) {
      if (err.status === 401) return;
      statsErrorEl.textContent = `Couldn't load the totals: ${err.message}. Refresh the page to try again.`;
      statsErrorEl.classList.remove('hidden');
    }
  }

  ready.then(loadStats);
})();
