/* global fflate */
(function () {
  const drop = document.getElementById('drop');
  const fileInput = document.getElementById('file');
  const btnConvert = document.getElementById('btn-convert');
  const btnDownload = document.getElementById('btn-download');
  const status = document.getElementById('status');
  const preview = document.getElementById('preview');
  const titleInput = document.getElementById('title');
  const authorInput = document.getElementById('author');

  let selectedFile = null;
  let htmlOutput = '';

  function setStatus(msg, kind) {
    status.textContent = msg;
    status.className = kind || '';
  }

  function escapeHtml(s) {
    return String(s)
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;');
  }

  function mdToHtml(md) {
    const lines = md.replace(/\r\n/g, '\n').split('\n');
    const out = [];
    let inCode = false;
    let codeBuf = [];

    function flushCode() {
      if (!codeBuf.length) return;
      out.push('<pre><code>' + escapeHtml(codeBuf.join('\n')) + '</code></pre>');
      codeBuf = [];
    }

    for (const line of lines) {
      if (line.trim().startsWith('```')) {
        if (inCode) {
          flushCode();
          inCode = false;
        } else {
          inCode = true;
        }
        continue;
      }
      if (inCode) {
        codeBuf.push(line);
        continue;
      }
      if (/^#{1,3}\s+/.test(line)) {
        const level = line.match(/^#+/)[0].length;
        out.push('<h' + level + '>' + inlineMd(line.replace(/^#+\s+/, '')) + '</h' + level + '>');
        continue;
      }
      if (/^---+$/.test(line.trim())) {
        out.push('<hr />');
        continue;
      }
      if (!line.trim()) {
        out.push('');
        continue;
      }
      out.push('<p>' + inlineMd(line) + '</p>');
    }
    flushCode();
    return out.join('\n');
  }

  function inlineMd(text) {
    return escapeHtml(text)
      .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
      .replace(/\*(.+?)\*/g, '<em>$1</em>')
      .replace(/`([^`]+)`/g, '<code>$1</code>');
  }

  function txtToHtml(txt) {
    const blocks = txt.replace(/\r\n/g, '\n').split(/\n{2,}/);
    return blocks
      .map((block) => {
        const t = block.trim();
        if (!t) return '';
        if (t.startsWith('Chapter ') || /^CHAPTER\s+\d+/i.test(t)) {
          return '<h2>' + escapeHtml(t.split('\n')[0]) + '</h2><p>' +
            escapeHtml(t.split('\n').slice(1).join('\n').trim()).replace(/\n/g, '<br />') + '</p>';
        }
        return '<p>' + escapeHtml(t).replace(/\n/g, '<br />') + '</p>';
      })
      .filter(Boolean)
      .join('\n');
  }

  function wrapBook(title, author, body) {
    const safeTitle = escapeHtml(title || 'Ebook');
    const safeAuthor = escapeHtml(author || 'Anonymous');
    return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8" />
<title>${safeTitle}</title>
<style>
  body { max-width: 42rem; margin: 2rem auto; padding: 0 1rem; font-family: Georgia, serif; line-height: 1.65; color: #1a1512; }
  h1 { font-family: system-ui, sans-serif; border-bottom: 1px solid #ccc; padding-bottom: 0.35rem; }
  .meta { color: #666; font-style: italic; margin-bottom: 2rem; }
  pre { background: #f4f0ea; padding: 0.75rem; overflow-x: auto; }
  code { font-family: ui-monospace, monospace; font-size: 0.92em; }
</style>
</head>
<body>
<h1>${safeTitle}</h1>
<p class="meta">by ${safeAuthor}</p>
${body}
</body>
</html>`;
  }

  async function epubToHtml(buffer) {
    const files = fflate.unzipSync(new Uint8Array(buffer));
    const textDecoder = new TextDecoder('utf-8');
    let rootfile = null;

    const container = files['META-INF/container.xml'];
    if (container) {
      const xml = textDecoder.decode(container);
      const m = xml.match(/full-path="([^"]+\.opf)"/i);
      if (m) rootfile = m[1];
    }
    if (!rootfile) {
      rootfile = Object.keys(files).find((k) => k.endsWith('.opf'));
    }
    if (!rootfile || !files[rootfile]) {
      throw new Error('EPUB package document (.opf) not found');
    }

    const opf = textDecoder.decode(files[rootfile]);
    const opfDir = rootfile.includes('/') ? rootfile.replace(/[^/]+$/, '') : '';
    const parser = new DOMParser();

    const opfDoc = parser.parseFromString(opf, 'application/xml');
    const manifest = {};
    opfDoc.querySelectorAll('package manifest item, manifest item, item').forEach((item) => {
      if (item.parentElement && item.parentElement.localName !== 'manifest') return;
      const id = item.getAttribute('id');
      const href = item.getAttribute('href');
      const type = item.getAttribute('media-type') || '';
      if (id && href) manifest[id] = { href, type };
    });

    const spine = [];
    opfDoc.querySelectorAll('package spine itemref, spine itemref, itemref').forEach((ref) => {
      if (ref.parentElement && ref.parentElement.localName !== 'spine') return;
      const idref = ref.getAttribute('idref');
      if (idref && manifest[idref]) spine.push(manifest[idref]);
    });

    const metaTitle = opfDoc.querySelector('metadata title, title');
    const metaCreator = opfDoc.querySelector('metadata creator, creator');
    if (metaTitle && !titleInput.value) titleInput.value = metaTitle.textContent.trim();
    if (metaCreator && !authorInput.value) authorInput.value = metaCreator.textContent.trim();

    const parts = [];
    for (const item of spine) {
      if (!/html|xhtml|xml/i.test(item.type) && !item.href.endsWith('.xhtml') && !item.href.endsWith('.html')) {
        continue;
      }
      const path = opfDir + item.href;
      const normalized = path.replace(/^\.\//, '');
      const alt = Object.keys(files).find((k) => k.endsWith(item.href) || k === normalized);
      const key = files[normalized] ? normalized : alt;
      if (!key || !files[key]) continue;
      const raw = textDecoder.decode(files[key]);
      const doc = parser.parseFromString(raw, 'text/html');
      const title = doc.querySelector('title');
      if (title && title.textContent.trim()) {
        parts.push('<h2>' + escapeHtml(title.textContent.trim()) + '</h2>');
      }
      const body = doc.querySelector('body');
      if (body) {
        parts.push('<div class="chapter">' + body.innerHTML + '</div>');
      }
    }

    if (!parts.length) throw new Error('No readable chapters found in EPUB');
    return parts.join('\n');
  }

  async function convert() {
    if (!selectedFile) return;
    setStatus('Converting…');
    const name = selectedFile.name.toLowerCase();
    const title = titleInput.value.trim() || selectedFile.name.replace(/\.[^.]+$/, '');
    const author = authorInput.value.trim() || 'Anonymous';
    let body = '';

    try {
      if (name.endsWith('.epub')) {
        const buf = await selectedFile.arrayBuffer();
        body = await epubToHtml(buf);
      } else if (name.endsWith('.md') || name.endsWith('.markdown')) {
        const text = await selectedFile.text();
        body = mdToHtml(text);
      } else {
        const text = await selectedFile.text();
        body = txtToHtml(text);
      }
      htmlOutput = wrapBook(title, author, body);
      preview.innerHTML = body;
      btnDownload.disabled = false;
      setStatus('Converted successfully', 'ok');
    } catch (err) {
      setStatus(err.message || String(err), 'err');
    }
  }

  function pickFile(file) {
    selectedFile = file;
    btnConvert.disabled = false;
    if (!titleInput.value) {
      titleInput.value = file.name.replace(/\.[^.]+$/, '').replace(/[-_]/g, ' ');
    }
    setStatus('Selected: ' + file.name);
  }

  drop.addEventListener('click', () => fileInput.click());
  fileInput.addEventListener('change', () => {
    if (fileInput.files[0]) pickFile(fileInput.files[0]);
  });
  drop.addEventListener('dragover', (e) => {
    e.preventDefault();
    drop.classList.add('drag');
  });
  drop.addEventListener('dragleave', () => drop.classList.remove('drag'));
  drop.addEventListener('drop', (e) => {
    e.preventDefault();
    drop.classList.remove('drag');
    if (e.dataTransfer.files[0]) pickFile(e.dataTransfer.files[0]);
  });

  btnConvert.addEventListener('click', convert);
  btnDownload.addEventListener('click', () => {
    if (!htmlOutput) return;
    const blob = new Blob([htmlOutput], { type: 'text/html;charset=utf-8' });
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = (titleInput.value.trim() || 'ebook').replace(/\s+/g, '-').toLowerCase() + '.html';
    a.click();
    URL.revokeObjectURL(a.href);
    setStatus('Download started', 'ok');
  });
})();
