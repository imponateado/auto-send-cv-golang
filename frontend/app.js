const apiBaseInput = document.getElementById('apiBase');
const apiBaseStatus = document.getElementById('apiBaseStatus');

apiBaseInput.value = localStorage.getItem('apiBase') || 'http://localhost:8080';
apiBaseInput.addEventListener('change', () => {
  localStorage.setItem('apiBase', apiBaseInput.value.trim());
  checkHealth();
});

function apiBase() {
  return apiBaseInput.value.trim().replace(/\/$/, '');
}

async function api(path, options = {}) {
  const res = await fetch(apiBase() + path, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  });
  const isJson = (res.headers.get('content-type') || '').includes('application/json');
  const data = isJson ? await res.json().catch(() => null) : await res.blob();
  if (!res.ok) {
    const message = isJson && data && data.error ? data.error : `HTTP ${res.status}`;
    throw new Error(message);
  }
  return data;
}

async function checkHealth() {
  try {
    await fetch(apiBase() + '/api/v1/whatsapp/groups/status');
    apiBaseStatus.className = 'dot online';
  } catch {
    apiBaseStatus.className = 'dot offline';
  }
}
checkHealth();
setInterval(checkHealth, 15000);

// ---------- Tabs ----------
document.querySelectorAll('.tab-btn').forEach((btn) => {
  btn.addEventListener('click', () => {
    document.querySelectorAll('.tab-btn').forEach((b) => b.classList.remove('active'));
    document.querySelectorAll('.tab-panel').forEach((p) => p.classList.remove('active'));
    btn.classList.add('active');
    document.getElementById(btn.dataset.tab).classList.add('active');
  });
});

// ---------- Render helpers ----------
function renderMsg(el, text, type = 'info') {
  el.innerHTML = `<div class="msg ${type}">${escapeHtml(text)}</div>`;
}

function renderJson(el, data) {
  el.innerHTML = `<pre class="json">${escapeHtml(JSON.stringify(data, null, 2))}</pre>`;
}

function escapeHtml(str) {
  return String(str).replace(/[&<>"']/g, (c) => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
  }[c]));
}

function fileToBase64(file) {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result);
    reader.onerror = reject;
    reader.readAsDataURL(file);
  });
}

async function handle(el, fn) {
  try {
    renderMsg(el, 'Carregando...', 'info');
    await fn();
  } catch (err) {
    renderMsg(el, err.message, 'error');
  }
}

// ---------- Vagas ----------
document.getElementById('clearBtn').addEventListener('click', async () => {
  const el = document.getElementById('clearResult');
  await handle(el, async () => {
    const data = await api('/api/v1/vacancies/clear', { method: 'POST' });
    renderMsg(el, data.message || 'Vagas limpas com sucesso', 'success');
  });
});

async function loadVacancies() {
  const el = document.getElementById('listVacanciesResult');
  await handle(el, async () => {
    const data = await api('/api/v1/vacancies');
    if (!data || !data.length) {
      renderMsg(el, 'Nenhuma vaga encontrada', 'info');
      return;
    }
    const rows = data.map((v) => `<tr>
      <td>${escapeHtml(v.index)}</td>
      <td>${escapeHtml(v.text)}</td>
      <td><button class="danger vacancy-delete-btn" data-id="${escapeHtml(v.id)}">Deletar</button></td>
    </tr>`).join('');
    el.innerHTML = `<table><thead><tr><th>Índice</th><th>Texto</th><th>Ações</th></tr></thead><tbody>${rows}</tbody></table>`;
    el.querySelectorAll('.vacancy-delete-btn').forEach((btn) => {
      btn.addEventListener('click', async () => {
        await handle(el, async () => {
          await api(`/api/v1/vacancies/${encodeURIComponent(btn.dataset.id)}`, { method: 'DELETE' });
          await loadVacancies();
        });
      });
    });
  });
}

document.getElementById('listVacanciesBtn').addEventListener('click', loadVacancies);

// ---------- Match ----------
document.getElementById('matchForm').addEventListener('submit', async (e) => {
  e.preventDefault();
  const el = document.getElementById('matchResult');
  await handle(el, async () => {
    const file = document.getElementById('matchFile').files[0];
    if (!file) throw new Error('Selecione um arquivo PDF');
    const fileBase64 = await fileToBase64(file);
    const data = await api('/api/v1/match', {
      method: 'POST',
      body: JSON.stringify({
        file_base64: fileBase64,
        candidate_email: document.getElementById('matchEmail').value,
        candidate_phone: document.getElementById('matchPhone').value,
      }),
    });
    renderJson(el, data);
  });
});

async function loadMatches() {
  const el = document.getElementById('matchesListResult');
  await handle(el, async () => {
    const data = await api('/api/v1/matches');
    if (!data || !data.length) {
      renderMsg(el, 'Nenhum match no histórico', 'info');
      return;
    }
    const rows = data.map((m) => `<tr>
      <td>${escapeHtml(m.id)}</td>
      <td>${escapeHtml(new Date(m.created_at).toLocaleString())}</td>
      <td>${escapeHtml(m.result ? m.result.status : '-')}</td>
      <td>${escapeHtml(m.result && m.result.matches ? m.result.matches.length : 0)}</td>
      <td><button class="danger match-delete-btn" data-id="${escapeHtml(m.id)}">Deletar</button></td>
    </tr>`).join('');
    el.innerHTML = `<table><thead><tr><th>ID</th><th>Data</th><th>Status</th><th>Matches</th><th>Ações</th></tr></thead><tbody>${rows}</tbody></table>`;
    el.querySelectorAll('.match-delete-btn').forEach((btn) => {
      btn.addEventListener('click', async () => {
        await handle(el, async () => {
          await api(`/api/v1/matches/${encodeURIComponent(btn.dataset.id)}`, { method: 'DELETE' });
          await loadMatches();
        });
      });
    });
  });
}

document.getElementById('matchesListBtn').addEventListener('click', loadMatches);

// ---------- Credenciais ----------
document.getElementById('credsForm').addEventListener('submit', async (e) => {
  e.preventDefault();
  const el = document.getElementById('credsResult');
  await handle(el, async () => {
    const data = await api('/api/v1/credentials', {
      method: 'POST',
      body: JSON.stringify({
        email: document.getElementById('credsEmail').value,
        provider: document.getElementById('credsProvider').value,
        client_id: document.getElementById('credsClientId').value,
        client_secret: document.getElementById('credsClientSecret').value,
        refresh_token: document.getElementById('credsRefreshToken').value,
      }),
    });
    renderMsg(el, data.message || 'Credenciais salvas', 'success');
    e.target.reset();
    document.getElementById('credsProvider').value = 'google';
  });
});

document.getElementById('credsDeleteForm').addEventListener('submit', async (e) => {
  e.preventDefault();
  const el = document.getElementById('credsDeleteResult');
  await handle(el, async () => {
    const email = document.getElementById('credsDeleteEmail').value;
    const data = await api(`/api/v1/credentials/${encodeURIComponent(email)}`, { method: 'DELETE' });
    renderMsg(el, data.message || 'Credenciais removidas', 'success');
  });
});

document.getElementById('credsListBtn').addEventListener('click', async () => {
  const el = document.getElementById('credsListResult');
  await handle(el, async () => {
    const data = await api('/api/v1/credentials');
    if (!data || !data.length) {
      renderMsg(el, 'Nenhuma credencial cadastrada', 'info');
      return;
    }
    const rows = data.map((c) => `<tr><td>${escapeHtml(c.email)}</td><td>${escapeHtml(c.provider)}</td></tr>`).join('');
    el.innerHTML = `<table><thead><tr><th>Email</th><th>Provedor</th></tr></thead><tbody>${rows}</tbody></table>`;
  });
});

// ---------- WhatsApp ----------
async function loadWappConnections() {
  const el = document.getElementById('wappListResult');
  await handle(el, async () => {
    const data = await api('/api/v1/whatsapp/connections');
    if (!data || !data.length) {
      renderMsg(el, 'Nenhuma conexão ativa', 'info');
      return;
    }
    const rows = data.map((c) => `<tr>
      <td>${escapeHtml(c.phone)}</td>
      <td>${escapeHtml(c.status)}</td>
      <td>${escapeHtml(c.jid || '-')}</td>
      <td><button class="danger wapp-disconnect-btn" data-phone="${escapeHtml(c.phone)}">Desconectar</button></td>
    </tr>`).join('');
    el.innerHTML = `<table><thead><tr><th>Telefone</th><th>Status</th><th>JID</th><th>Ações</th></tr></thead><tbody>${rows}</tbody></table>`;
    el.querySelectorAll('.wapp-disconnect-btn').forEach((btn) => {
      btn.addEventListener('click', async () => {
        await handle(el, async () => {
          await api(`/api/v1/whatsapp/disconnect?phone=${encodeURIComponent(btn.dataset.phone)}`, { method: 'POST' });
          await loadWappConnections();
        });
      });
    });
  });
}

document.getElementById('wappListBtn').addEventListener('click', loadWappConnections);

document.getElementById('wappQrForm').addEventListener('submit', async (e) => {
  e.preventDefault();
  const el = document.getElementById('wappQrResult');
  await handle(el, async () => {
    const phone = document.getElementById('wappQrPhone').value;
    const res = await fetch(`${apiBase()}/api/v1/whatsapp/qr?phone=${encodeURIComponent(phone)}`);
    const contentType = res.headers.get('content-type') || '';
    if (!res.ok) {
      const data = await res.json().catch(() => null);
      throw new Error((data && data.error) || `HTTP ${res.status}`);
    }
    if (contentType.includes('image/png')) {
      const blob = await res.blob();
      const url = URL.createObjectURL(blob);
      el.innerHTML = `<p>Escaneie o QR Code com o WhatsApp:</p><img class="qr-img" src="${url}" alt="QR Code">`;
    } else {
      const data = await res.json();
      renderMsg(el, data.message || 'OK', 'success');
    }
  });
});

// ---------- Grupos ----------
function groupsPhone() {
  const phone = document.getElementById('groupsPhone').value.trim();
  if (!phone) throw new Error('Informe o telefone');
  return phone;
}

function renderGroups(el, groups, watchedJids) {
  if (!groups || !groups.length) {
    renderMsg(el, 'Nenhum grupo encontrado', 'info');
    return;
  }
  const rows = groups.map((g) => {
    const checked = watchedJids ? watchedJids.has(g.jid) : false;
    return `<div class="group-row">
      <input type="checkbox" data-jid="${escapeHtml(g.jid)}" data-name="${escapeHtml(g.name)}" ${watchedJids ? (checked ? 'checked' : '') : 'disabled checked'}>
      <span>${escapeHtml(g.name)}</span>
    </div>`;
  }).join('');
  const saveBtn = watchedJids ? '<button id="groupsSaveBtn">Salvar seleção</button>' : '';
  el.innerHTML = `<div class="group-list">${rows}</div>${saveBtn}`;
  if (watchedJids) {
    document.getElementById('groupsSaveBtn').addEventListener('click', () => saveWatchedGroups(el));
  }
}

async function saveWatchedGroups(el) {
  const selected = [...el.querySelectorAll('input[type=checkbox]:checked')].map((cb) => ({
    jid: cb.dataset.jid,
    name: cb.dataset.name,
  }));
  await handle(el, async () => {
    const phone = groupsPhone();
    await api(`/api/v1/whatsapp/groups/watched?phone=${encodeURIComponent(phone)}`, {
      method: 'PUT',
      body: JSON.stringify({ groups: selected }),
    });
    renderMsg(el, 'Grupos monitorados atualizados', 'success');
  });
}

document.getElementById('groupsListBtn').addEventListener('click', async () => {
  const el = document.getElementById('groupsList');
  await handle(el, async () => {
    const phone = groupsPhone();
    const [groups, watched] = await Promise.all([
      api(`/api/v1/whatsapp/groups?phone=${encodeURIComponent(phone)}`),
      api(`/api/v1/whatsapp/groups/watched?phone=${encodeURIComponent(phone)}`),
    ]);
    const watchedJids = new Set((watched || []).map((w) => w.group_jid));
    renderGroups(el, groups, watchedJids);
  });
});

document.getElementById('groupsWatchedBtn').addEventListener('click', async () => {
  const el = document.getElementById('groupsList');
  await handle(el, async () => {
    const phone = groupsPhone();
    const watched = await api(`/api/v1/whatsapp/groups/watched?phone=${encodeURIComponent(phone)}`);
    if (!watched || !watched.length) {
      renderMsg(el, 'Nenhum grupo monitorado para este telefone', 'info');
      return;
    }
    const rows = watched.map((w) => `<tr><td>${escapeHtml(w.group_name)}</td><td>${escapeHtml(w.group_jid)}</td></tr>`).join('');
    el.innerHTML = `<table><thead><tr><th>Nome</th><th>JID</th></tr></thead><tbody>${rows}</tbody></table>`;
  });
});

document.getElementById('groupsUnwatchAllBtn').addEventListener('click', async () => {
  const el = document.getElementById('groupsList');
  await handle(el, async () => {
    const phone = groupsPhone();
    const data = await api(`/api/v1/whatsapp/groups/watched?phone=${encodeURIComponent(phone)}`, { method: 'DELETE' });
    renderMsg(el, data.message || 'Grupos desmonitorados com sucesso', 'success');
  });
});

document.getElementById('watchedPhonesBtn').addEventListener('click', async () => {
  const el = document.getElementById('watchedPhonesResult');
  await handle(el, async () => {
    const data = await api('/api/v1/whatsapp/groups/watched-phones');
    if (!data || !data.length) {
      renderMsg(el, 'Nenhum telefone monitorando grupos', 'info');
      return;
    }
    const rows = data.map((phone) => `<tr><td>${escapeHtml(phone)}</td></tr>`).join('');
    el.innerHTML = `<table><thead><tr><th>Telefone</th></tr></thead><tbody>${rows}</tbody></table>`;
  });
});

document.getElementById('flushBtn').addEventListener('click', async () => {
  const el = document.getElementById('bufferResult');
  await handle(el, async () => {
    const data = await api('/api/v1/whatsapp/groups/flush', { method: 'POST' });
    renderMsg(el, `Processados ${data.items_processed} itens`, 'success');
  });
});

document.getElementById('statusBtn').addEventListener('click', async () => {
  const el = document.getElementById('bufferResult');
  await handle(el, async () => {
    const data = await api('/api/v1/whatsapp/groups/status');
    renderJson(el, data);
  });
});
