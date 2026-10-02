// Shared code for every dashboard page: checks the session, fills in the
// header, wires up logout, and provides small helpers as window.KS.
//
// The session check only decides what the browser shows. The real
// protection is on the server, where every admin endpoint checks the
// session itself.
(function () {
  function toLogin() {
    location.replace('/');
  }

  class ApiError extends Error {
    constructor(message, status, field) {
      super(message);
      this.status = status;
      this.field = field;
    }
  }

  // Calls a KeyShin admin endpoint. Throws ApiError with the server's
  // message; sends the admin to the login page when the session has ended.
  async function api(path, { method = 'GET', body } = {}) {
    const opts = { method, credentials: 'same-origin', headers: {} };
    if (body !== undefined) {
      opts.headers['Content-Type'] = 'application/json';
      opts.body = JSON.stringify(body);
    }
    let res;
    try {
      res = await fetch(path, opts);
    } catch {
      throw new ApiError('Could not reach the server. Check your connection and try again.', 0);
    }
    if (res.status === 401) {
      toLogin();
      throw new ApiError('Your session has ended.', 401);
    }
    const data = await res.json().catch(() => null);
    if (!res.ok) {
      throw new ApiError((data && data.error) || `Request failed (error ${res.status}).`, res.status, data && data.field);
    }
    return data;
  }

  function escapeHTML(value) {
    return String(value ?? '').replace(/[&<>"']/g, (c) => ({
      '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
    }[c]));
  }

  const dateFmt = new Intl.DateTimeFormat(undefined, { day: 'numeric', month: 'short', year: 'numeric' });
  const dateTimeFmt = new Intl.DateTimeFormat(undefined, { day: 'numeric', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' });

  function formatDate(iso) {
    return iso ? dateFmt.format(new Date(iso)) : '';
  }

  function formatDateTime(iso) {
    return iso ? dateTimeFmt.format(new Date(iso)) : '';
  }

  // "2026-10-31" from a date input → the end of that day in local time, as ISO.
  function endOfDayISO(dateValue) {
    if (!dateValue) return null;
    return new Date(`${dateValue}T23:59:59`).toISOString();
  }

  // ISO timestamp → "2026-10-31" (local) for a date input.
  function toDateInput(iso) {
    if (!iso) return '';
    const d = new Date(iso);
    const pad = (n) => String(n).padStart(2, '0');
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
  }

  let toastTimer;
  function toast(message, kind = 'ok') {
    let el = document.getElementById('toast');
    if (!el) {
      el = document.createElement('div');
      el.id = 'toast';
      el.setAttribute('role', 'status');
      el.setAttribute('aria-live', 'polite');
      document.body.appendChild(el);
    }
    el.textContent = message;
    el.dataset.kind = kind;
    el.classList.add('show');
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => el.classList.remove('show'), 3200);
  }

  async function copy(text, what = 'Copied') {
    try {
      await navigator.clipboard.writeText(text);
      toast(`${what} to clipboard.`);
    } catch {
      toast('Copying failed. Select the text and copy it manually.', 'error');
    }
  }

  // Shows field-level errors inside a form. Inputs are matched by name.
  function showFormError(form, err) {
    clearFormError(form);
    const box = form.querySelector('[data-form-error]');
    const field = err.field && form.querySelector(`[name="${err.field}"]`);
    if (field) {
      field.setAttribute('aria-invalid', 'true');
      field.focus();
    }
    if (box) {
      box.textContent = err.message;
      box.hidden = false;
    }
  }

  function clearFormError(form) {
    form.querySelectorAll('[aria-invalid]').forEach((el) => el.removeAttribute('aria-invalid'));
    const box = form.querySelector('[data-form-error]');
    if (box) {
      box.hidden = true;
      box.textContent = '';
    }
  }

  function busy(button, isBusy, busyText) {
    if (isBusy) {
      button.dataset.label = button.textContent;
      button.textContent = busyText;
      button.disabled = true;
      button.setAttribute('aria-busy', 'true');
    } else {
      button.textContent = button.dataset.label || button.textContent;
      button.disabled = false;
      button.removeAttribute('aria-busy');
    }
  }

  const stateLabels = { active: 'Active', expired: 'Expired', revoked: 'Revoked' };
  function statePill(state) {
    return `<span class="pill" data-state="${escapeHTML(state)}">${stateLabels[state] || escapeHTML(state)}</span>`;
  }

  // Fills in the header. It runs alongside the page's own data requests;
  // whichever gets a 401 first sends the admin to the login page.
  const ready = api('/api/session').then((admin) => {
    const userEl = document.getElementById('session-user');
    if (userEl) userEl.textContent = admin.username;
    return admin;
  }).catch(() => {
    toLogin();
    return new Promise(() => {}); // never resolves; we're leaving the page
  });

  // Placeholder rows shown while a table loads, so the page doesn't jump.
  function skeletonRows(columns, rows = 3) {
    const cell = '<td><span class="skeleton"></span></td>';
    return Array.from({ length: rows }, () => `<tr aria-hidden="true">${cell.repeat(columns)}</tr>`).join('');
  }

  document.getElementById('logout')?.addEventListener('click', async (event) => {
    const button = event.currentTarget;
    busy(button, true, 'Logging out…');
    try {
      await fetch('/api/logout', { method: 'POST', credentials: 'same-origin' });
    } finally {
      toLogin();
    }
  });

  window.KS = {
    api, ApiError, ready, escapeHTML, formatDate, formatDateTime, endOfDayISO, toDateInput,
    toast, copy, showFormError, clearFormError, busy, statePill, skeletonRows,
  };
})();
