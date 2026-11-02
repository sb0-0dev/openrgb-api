const form = document.querySelector('#config-form');
const status = document.querySelector('#status');
const message = document.querySelector('#message');
const configPath = document.querySelector('#config-path');
const controllers = document.querySelector('#controllers');
const pairingStatus = document.querySelector('#pairing-status');
const pairingCodePanel = document.querySelector('#pairing-code-panel');
const pairedDevices = document.querySelector('#paired-devices');

/**
 * Render a connection status without exposing server internals to the browser.
 * @param {string} text - Status text to display.
 * @param {boolean} [connected=false] - Whether to style the status as connected.
 * @returns {void}
 */
function setStatus(text, connected = false) {
  status.textContent = text;
  status.classList.toggle('connected', connected);
}

/**
 * Fetch a JSON API endpoint and surface API failures as errors so the page can display them.
 * @param {string} url - API path to request.
 * @param {RequestInit} [options] - Optional fetch options such as method, headers, and body.
 * @returns {Promise<any>} The parsed JSON response body.
 * @throws {Error} If the response is not OK; the message is the API's `error` field when present.
 */
async function requestJSON(url, options) {
  const response = await fetch(url, options);
  const body = await response.json();
  if (!response.ok) {
    throw new Error(body.error || `Request failed with status ${response.status}`);
  }
  return body;
}

/**
 * Load the persisted settings into the form controls.
 * @returns {Promise<void>}
 */
async function loadConfig() {
  const config = await requestJSON('/api/config');
  form.elements.listen_address.value = config.server.listen_address;
  form.elements.admin_address.value = config.server.admin_address || '';
  form.elements.public_url.value = config.server.public_url || '';
  form.elements.host.value = config.openrgb.host;
  form.elements.port.value = config.openrgb.port;
  form.elements.client_name.value = config.openrgb.client_name;
  configPath.textContent = config.path;
}

/**
 * @typedef {Object} ControllerSummary
 * @property {number} id - Controller index on the OpenRGB server.
 * @property {string} name - Controller display name.
 * @property {string} [vendor] - Hardware vendor.
 * @property {string} [model] - Hardware model.
 * @property {number} modes - Number of supported modes.
 * @property {number} zones - Number of zones.
 * @property {number} leds - Number of LEDs.
 */

/**
 * Show controller summaries returned by the OpenRGB client.
 * @param {ControllerSummary[]} items - Controllers to render.
 * @returns {void}
 */
function renderControllers(items) {
  if (items.length === 0) {
    controllers.innerHTML = '<p class="muted">No controllers found.</p>';
    return;
  }
  controllers.innerHTML = items.map((controller) => `
    <article class="controller">
      <div>
        <h3>${escapeHTML(controller.name)}</h3>
        <p>${escapeHTML(controller.vendor || 'Unknown vendor')} ${escapeHTML(controller.model || '')}</p>
      </div>
      <dl>
        <div><dt>LEDs</dt><dd>${controller.leds}</dd></div>
        <div><dt>Zones</dt><dd>${controller.zones}</dd></div>
        <div><dt>Modes</dt><dd>${controller.modes}</dd></div>
      </dl>
    </article>
  `).join('');
}

/**
 * Refresh controller summaries independently from configuration loading.
 * Errors are rendered in the page rather than thrown.
 * @returns {Promise<void>}
 */
async function loadControllers() {
  controllers.textContent = 'Loading controllers...';
  try {
    renderControllers(await requestJSON('/api/controllers'));
    setStatus('OpenRGB connected', true);
  } catch (error) {
    controllers.innerHTML = `<p class="error">${escapeHTML(error.message)}</p>`;
    setStatus('OpenRGB unavailable');
  }
}

/**
 * @typedef {Object} PairedDevice
 * @property {string} id - Unique device identifier used for revocation.
 * @property {string} name - Device name supplied during pairing.
 * @property {string} created_at - ISO timestamp of when the device was paired.
 */

/**
 * Render the list of paired mobile devices with a revoke button for each.
 * @param {PairedDevice[]} items - Paired devices to render.
 * @returns {void}
 */
function renderPairedDevices(items) {
  if (items.length === 0) {
    pairedDevices.innerHTML = '<p class="muted">No mobile devices paired.</p>';
    return;
  }
  pairedDevices.innerHTML = items.map((device) => `
    <div class="paired-device">
      <div>
        <strong>${escapeHTML(device.name)}</strong>
        <small>Paired ${escapeHTML(new Date(device.created_at).toLocaleString())}</small>
      </div>
      <button class="secondary" type="button" data-revoke-device="${escapeHTML(device.id)}">Revoke</button>
    </div>
  `).join('');
}

/**
 * Fetch and render paired devices. Only the host computer may list them, so
 * failures are shown as a hint to open the page on that computer.
 * @returns {Promise<void>}
 */
async function loadPairedDevices() {
  try {
    const response = await requestJSON('/api/pairing/devices');
    renderPairedDevices(response.devices);
  } catch (error) {
    pairedDevices.innerHTML = `<p class="muted">${escapeHTML(error.message)}. Open this page on the OpenRGB computer to manage pairing.</p>`;
  }
}

/**
 * Start mobile pairing: request a single-use code and show it, plus a QR code when available.
 * @param {MouseEvent} event - Click event from the start pairing button.
 */
document.querySelector('#start-pairing').addEventListener('click', async (event) => {
  const button = event.currentTarget;
  button.disabled = true;
  pairingStatus.textContent = 'Generating code...';
  try {
    const urlSelect = document.querySelector('#pairing-url');
    const pairing = await requestJSON('/api/pairing/start', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ server_url: urlSelect.value }),
    });
    document.querySelector('#pairing-code').textContent = pairing.code;
    document.querySelector('#pairing-expiry').textContent = `Expires ${new Date(pairing.expires_at).toLocaleTimeString()}`;
    const urls = pairing.server_urls || [];
    urlSelect.replaceChildren(...urls.map((url) => {
      const option = new Option(url, url);
      option.selected = url === pairing.server_url;
      return option;
    }));
    document.querySelector('#pairing-url-label').hidden = urls.length < 2;
    const pairingQR = document.querySelector('#pairing-qr');
    const pairingQRHint = document.querySelector('#pairing-qr-hint');
    pairingQR.hidden = !pairing.qr_image;
    pairingQRHint.hidden = Boolean(pairing.qr_image);
    if (pairing.qr_image) pairingQR.src = pairing.qr_image;
    pairingCodePanel.hidden = false;
    pairingStatus.textContent = pairing.qr_image ? 'Scan with the mobile app. The code can be used once.' : 'Enter this code in the mobile app. It can be used once.';
  } catch (error) {
    pairingStatus.textContent = error.message;
  } finally {
    button.disabled = false;
  }
});

/**
 * Revoke a paired device when its revoke button is clicked (event delegation).
 * @param {MouseEvent} event - Click event from the paired devices list.
 */
pairedDevices.addEventListener('click', async (event) => {
  const button = event.target.closest('[data-revoke-device]');
  if (!button) return;
  button.disabled = true;
  try {
    const response = await fetch(`/api/pairing/devices/${encodeURIComponent(button.dataset.revokeDevice)}`, { method: 'DELETE' });
    if (!response.ok) {
      const body = await response.json();
      throw new Error(body.error || `Request failed with status ${response.status}`);
    }
    pairingStatus.textContent = 'Device access revoked.';
    await loadPairedDevices();
  } catch (error) {
    pairingStatus.textContent = error.message;
    button.disabled = false;
  }
});

/**
 * Save the form as JSON; the server persists it as TOML.
 * @param {SubmitEvent} event - Submit event from the configuration form.
 */
form.addEventListener('submit', async (event) => {
  event.preventDefault();
  message.textContent = 'Saving...';
  try {
    const config = {
      server: {
        listen_address: form.elements.listen_address.value.trim(),
        admin_address: form.elements.admin_address.value.trim(),
        public_url: form.elements.public_url.value.trim(),
      },
      openrgb: {
        host: form.elements.host.value.trim(),
        port: form.elements.port.value.trim(),
        client_name: form.elements.client_name.value.trim(),
      },
    };
    const saved = await requestJSON('/api/config', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(config),
    });
    configPath.textContent = saved.path;
    message.textContent = 'Saved. Restart the service to apply connection changes.';
  } catch (error) {
    message.textContent = error.message;
  }
});

document.querySelector('#refresh').addEventListener('click', loadControllers);

/**
 * Escape server-provided text before placing it into an HTML template.
 * @param {unknown} value - Value to convert to a string and escape.
 * @returns {string} The HTML-escaped string.
 */
function escapeHTML(value) {
  return String(value).replace(/[&<>'"]/g, (character) => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;'
  }[character]));
}

Promise.all([loadConfig(), loadControllers(), loadPairedDevices()]).catch((error) => {
  message.textContent = error.message;
});
