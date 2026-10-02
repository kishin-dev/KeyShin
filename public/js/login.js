// Login page: posts credentials to /api/login and goes to the dashboard.
(function () {
  const form = document.getElementById('login-form');
  const userEl = document.getElementById('username');
  const passEl = document.getElementById('password');
  const errorEl = document.getElementById('login-error');
  const submitEl = document.getElementById('login-submit');
  const toggleEl = document.getElementById('toggle-password');

  // Already signed in? Skip the form.
  fetch('/api/session', { credentials: 'same-origin' })
    .then((res) => { if (res.ok) location.replace('/dashboard'); })
    .catch(() => {});

  toggleEl.addEventListener('click', () => {
    const show = passEl.type === 'password';
    passEl.type = show ? 'text' : 'password';
    toggleEl.textContent = show ? 'Hide' : 'Show';
    toggleEl.setAttribute('aria-pressed', String(show));
  });

  function showError(message, field) {
    errorEl.textContent = message;
    errorEl.classList.remove('hidden');
    [userEl, passEl].forEach((el) => el.removeAttribute('aria-invalid'));
    if (field) {
      field.setAttribute('aria-invalid', 'true');
      field.focus();
    }
  }

  function clearError() {
    errorEl.classList.add('hidden');
    [userEl, passEl].forEach((el) => el.removeAttribute('aria-invalid'));
  }

  form.addEventListener('submit', async (event) => {
    event.preventDefault();
    clearError();

    const username = userEl.value.trim();
    const password = passEl.value;
    if (!username) return showError('Enter your username.', userEl);
    if (!password) return showError('Enter your password.', passEl);

    submitEl.disabled = true;
    submitEl.setAttribute('aria-busy', 'true');
    submitEl.textContent = 'Signing in…';

    try {
      const res = await fetch('/api/login', {
        method: 'POST',
        credentials: 'same-origin',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username, password }),
      });

      if (res.ok) {
        location.replace('/dashboard');
        return;
      }
      if (res.status === 401) {
        passEl.value = '';
        showError('Username or password is incorrect.', passEl);
      } else {
        const data = await res.json().catch(() => ({}));
        showError(data.error ? `Sign-in failed: ${data.error}.` : `Sign-in failed (error ${res.status}). Try again.`);
      }
    } catch {
      showError('Could not reach the server. Check your connection and try again.');
    } finally {
      submitEl.disabled = false;
      submitEl.removeAttribute('aria-busy');
      submitEl.textContent = 'Sign in';
    }
  });
})();
