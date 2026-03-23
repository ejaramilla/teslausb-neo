// TeslaUSB Neo Web UI
fetch('/api/v1/status').then(r => r.json()).then(data => {
  document.querySelector('p').textContent = 'Status: ' + data.state;
});
