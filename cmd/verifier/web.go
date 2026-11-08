package main

const indexHTML = `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<title>Election Verifier</title>
<style>
  body { font-family: -apple-system, sans-serif; max-width: 800px; margin: 40px auto; padding: 20px; background: #f5f5f5; }
  h1 { color: #222; }
  .input-row { display: flex; gap: 10px; margin: 20px 0; }
  input { flex: 1; padding: 12px; font-size: 16px; border: 1px solid #ccc; border-radius: 4px; }
  button { padding: 12px 24px; font-size: 16px; background: #0066cc; color: white; border: none; border-radius: 4px; cursor: pointer; }
  button:hover { background: #0052a3; }
  .result { background: white; padding: 20px; border-radius: 8px; margin-top: 20px; box-shadow: 0 2px 8px rgba(0,0,0,0.1); }
  .verified { border-left: 4px solid #28a745; }
  .failed { border-left: 4px solid #dc3545; }
  .candidate { display: flex; justify-content: space-between; padding: 8px 0; border-bottom: 1px solid #eee; }
  .sum { font-weight: bold; color: #0066cc; }
  .check { color: #28a745; }
  .cross { color: #dc3545; }
</style>
</head>
<body>
<h1>Election Verifier</h1>
<p>Independently verify that a published election result is correct. This tool reads only public ledger data — no keys required.</p>

<div class="input-row">
  <input type="text" id="electionId" placeholder="Election ID (e.g. tally-test-...)" />
  <button onclick="verify()">Verify</button>
</div>

<div id="result"></div>

<script>
async function verify() {
  const id = document.getElementById('electionId').value.trim();
  if (!id) return;
  const resultEl = document.getElementById('result');
  resultEl.innerHTML = '<div class="result">Verifying...</div>';

  try {
    const resp = await fetch('/api/verify/' + encodeURIComponent(id));
    const data = await resp.json();
    render(data);
  } catch (e) {
    resultEl.innerHTML = '<div class="result failed">Error: ' + e + '</div>';
  }
}

function render(data) {
  const ok = data.proofsVerified && data.ciphertextMatch;
  const cls = ok ? 'verified' : 'failed';

  let html = '<div class="result ' + cls + '">';
  html += '<h2>' + (ok ? '✓ VERIFIED' : '✗ VERIFICATION FAILED') + '</h2>';
  html += '<p><b>Election:</b> ' + data.electionId + '</p>';
  html += '<p><b>Phase:</b> ' + data.phase + ' &nbsp; <b>Ballots cast:</b> ' + data.ballotsCast + '</p>';
  html += '<h3>Results</h3>';

  for (const r of data.candidateResults) {
    const mark = r.proofValid ? '<span class="check">✓</span>' : '<span class="cross">✗</span>';
    html += '<div class="candidate"><span>' + mark + ' ' + r.candidate + '</span><span class="sum">' + r.sum + '</span></div>';
  }

  html += '<p style="margin-top:20px">';
  html += '<b>ZK proofs:</b> ' + (data.proofsVerified ? '✓ all verified' : '✗ failed') + '<br>';
  html += '<b>Ciphertext recomputation:</b> ' + (data.ciphertextMatch ? '✓ matches on-chain tally' : '✗ mismatch');
  html += '</p>';

  if (data.errors && data.errors.length > 0) {
    html += '<p><b>Errors:</b></p><ul>';
    for (const e of data.errors) html += '<li>' + e + '</li>';
    html += '</ul>';
  }

  html += '</div>';
  document.getElementById('result').innerHTML = html;
}
</script>
</body>
</html>`
