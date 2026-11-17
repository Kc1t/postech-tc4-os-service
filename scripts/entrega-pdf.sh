#!/usr/bin/env bash
# Gera o PDF de entrega (ABNT NBR 14724) a partir do Markdown, com os diagramas Mermaid renderizados,
# usando o Chrome em modo headless (marked e mermaid carregados do jsDelivr na hora da impressão).
# Duas passagens: a primeira mede em que página cada seção cai, a segunda imprime o sumário com os números.
#
#   VIDEO_URL=https://youtu.be/xxxx GATEWAY_URL=https://... NR_URL=https://... \
#     scripts/entrega-pdf.sh [docs/DOCUMENTO_ENTREGA_FASE4.md] [saída.pdf]
set -euo pipefail

SRC=${1:-docs/DOCUMENTO_ENTREGA_FASE4.md}
OUT=${2:-${SRC%.md}.pdf}
CHROME=${CHROME:-"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"}
[ -x "$CHROME" ] || CHROME=$(command -v google-chrome || command -v chromium || true)
[ -n "$CHROME" ] || { echo "Chrome não encontrado (defina CHROME)" >&2; exit 1; }
grep -q '</script' "$SRC" && { echo "o Markdown não pode conter </script>" >&2; exit 1; }

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
SRCDIR=$(cd "$(dirname "$SRC")" && pwd)
case $OUT in /*) ABS=$OUT ;; *) ABS=$PWD/$OUT ;; esac

build() { # $1 = JSON {titulo: página}, $2 = pdf de saída
  local HTML="$TMP/entrega.html"
  {
    printf '<!doctype html>\n<html lang="pt-BR"><head><meta charset="utf-8"><base href="file://%s/">\n' "$SRCDIR"
    printf '<script>window.PAGES=%s;</script>\n' "$1"
    cat <<'HEAD'
<title>Tech Challenge Fase 4 · Documento de entrega</title>
<style>
  * { -webkit-print-color-adjust: exact; print-color-adjust: exact; }
  /* NBR 14724: margens 3 cm (esq. e sup.) e 2 cm (dir. e inf.); numeração no canto superior direito */
  @page { size: A4; margin: 30mm 20mm 20mm 30mm; @top-right { content: counter(page); font: 10pt Arial, Helvetica, sans-serif; } }
  @page semnumero { @top-right { content: none; } }
  body { font: 12pt/1.5 Arial, Helvetica, sans-serif; color: #000; text-align: justify; hyphens: auto; }
  p { margin: 0 0 0; text-indent: 12.5mm; }
  h1, h2, h3 { font-size: 12pt; line-height: 1.5; margin: 0; page-break-after: avoid; text-align: left; }
  h2 { text-transform: uppercase; font-weight: bold; margin-top: 18pt; margin-bottom: 6pt; }
  h3 { font-weight: bold; margin-top: 12pt; margin-bottom: 4pt; }
  ul, ol { margin: 4pt 0 4pt 0; padding-left: 12.5mm; } li { margin: 2pt 0; }
  blockquote { margin: 6pt 0 6pt 40mm; font-size: 10pt; line-height: 1.15; text-indent: 0; }
  blockquote p { text-indent: 0; }
  a { color: #000; text-decoration: underline; }
  code { font: 10pt "Courier New", monospace; }
  /* Tabelas no padrão IBGE: filetes horizontais, sem linhas verticais, fonte menor e espaço simples */
  table { border-collapse: collapse; width: 100%; margin: 6pt 0 10pt; font-size: 10pt; line-height: 1.2; text-align: left; border-top: 1.5px solid #000; border-bottom: 1.5px solid #000; page-break-inside: avoid; }
  th { border-bottom: 1px solid #000; padding: 4pt 6pt; vertical-align: bottom; }
  td { padding: 3pt 6pt; vertical-align: top; }
  td p, th p { text-indent: 0; }
  img { max-width: 100%; display: block; margin: 8pt auto 2pt; }
  .fonte { font-size: 10pt; line-height: 1.15; text-align: center; text-indent: 0; margin: 0 0 10pt; }
  pre.mermaid { text-align: center; page-break-inside: avoid; margin: 8pt 0 2pt; } pre.mermaid svg { max-width: 100%; height: auto; }
  /* Capa e folha de rosto */
  .capa, .rosto { page: semnumero; page-break-after: always; min-height: 245mm; display: flex; flex-direction: column; text-align: center; text-indent: 0; }
  .capa p, .rosto p { text-indent: 0; margin: 0; }
  .capa .topo, .rosto .topo { font-weight: bold; text-transform: uppercase; line-height: 1.5; }
  .capa .autores, .rosto .autores { margin-top: 18mm; line-height: 1.5; }
  .capa .titulo { margin-top: auto; margin-bottom: auto; font-weight: bold; text-transform: uppercase; font-size: 14pt; line-height: 1.5; }
  .capa .sub { font-weight: normal; text-transform: none; font-size: 12pt; display: block; margin-top: 6pt; }
  .capa .rodape, .rosto .rodape { margin-top: auto; font-weight: bold; line-height: 1.5; }
  .rosto .titulo { margin-top: 40mm; font-weight: bold; text-transform: uppercase; }
  .rosto .natureza { margin: 18mm 0 0 80mm; font-size: 10pt; line-height: 1.15; text-align: justify; }
  .rosto .video { margin-top: 14mm; font-size: 11pt; }
  /* Sumário */
  #sumario { page-break-after: always; text-align: left; }
  #sumario h2 { text-align: center; margin: 0 0 12pt; }
  #sumario .l { display: flex; align-items: baseline; margin: 0; line-height: 1.5; }
  #sumario .l .t { flex: 0 1 auto; } #sumario .l .d { flex: 1 1 auto; border-bottom: 1px dotted #000; margin: 0 4px; transform: translateY(-3px); } #sumario .l .p { flex: 0 0 auto; }
  #sumario .n3 { padding-left: 8mm; }
</style></head><body>
<main id="out"></main>
<script type="text/markdown" id="md">
HEAD
    sed -e "s|{{VIDEO_URL}}|${VIDEO_URL:-#}|g" -e "s|{{GATEWAY_URL}}|${GATEWAY_URL:-#}|g" -e "s|{{NR_URL}}|${NR_URL:-#}|g" "$SRC"
    cat <<'TAIL'
</script>
<script src="https://cdn.jsdelivr.net/npm/marked@12.0.2/marked.min.js"></script>
<script src="https://cdn.jsdelivr.net/npm/mermaid@10.9.1/dist/mermaid.min.js"></script>
<script>
  const out = document.getElementById('out');
  out.innerHTML = marked.parse(document.getElementById('md').textContent);
  // numeração ABNT: "1. Título" -> "1 Título"
  out.querySelectorAll('h2, h3').forEach(h => { h.textContent = h.textContent.replace(/^(\d+(\.\d+)*)\.\s+/, '$1 '); });
  // sumário (seções primárias e secundárias), com a página medida na primeira passagem
  const toc = document.getElementById('sumario');
  if (toc) {
    toc.innerHTML = '<h2>Sumário</h2>';
    out.querySelectorAll('h2, h3').forEach(h => {
      if (h.closest('#sumario')) return;
      const t = h.textContent.trim(), row = document.createElement('div');
      row.className = 'l ' + (h.tagName === 'H3' ? 'n3' : 'n2');
      const label = h.tagName === 'H2' ? t.toUpperCase() : t;
      row.innerHTML = '<span class="t"></span><span class="d"></span><span class="p"></span>';
      row.children[0].textContent = label; row.children[2].textContent = (window.PAGES || {})[t] || '';
      toc.appendChild(row);
    });
  }
  out.querySelectorAll('pre > code.language-mermaid').forEach(code => {
    const pre = document.createElement('pre'); pre.className = 'mermaid'; pre.textContent = code.textContent;
    code.parentElement.replaceWith(pre);
  });
  mermaid.initialize({ startOnLoad: false, theme: 'neutral', securityLevel: 'loose', flowchart: { htmlLabels: true } });
  mermaid.run();
</script>
</body></html>
TAIL
  } > "$HTML"
  "$CHROME" --headless=new --disable-gpu --no-pdf-header-footer --virtual-time-budget=20000 \
    --run-all-compositor-stages-before-draw --print-to-pdf="$2" "file://$HTML" 2>/dev/null
}

build '{}' "$TMP/p1.pdf"

# mede a página de cada seção (a partir da página 4, depois de capa, folha de rosto e sumário)
PAGES=$(python3 -I - "$TMP/p1.pdf" "$SRC" <<'PY'
import json, re, subprocess, sys
pdf, md = sys.argv[1], sys.argv[2]
n = int(re.search(r'Pages:\s+(\d+)', subprocess.run(['pdfinfo', pdf], capture_output=True, text=True).stdout).group(1))
pages = [re.sub(r'\s+', ' ', subprocess.run(['pdftotext', '-f', str(i), '-l', str(i), pdf, '-'], capture_output=True, text=True).stdout).lower() for i in range(1, n + 1)]
res, cur = {}, 4
for line in open(md, encoding='utf-8'):
    m = re.match(r'(#{2,3}) (.+)', line)
    if not m: continue
    t = m.group(2).strip(); t2 = re.sub(r'^(\d+(\.\d+)*)\.\s+', r'\1 ', t)
    key = re.sub(r'\s+', ' ', re.sub(r'[`*_]', '', t2)).lower()
    for p in range(cur, n + 1):
        if key in pages[p - 1]: res[t2] = p; cur = p; break
print(json.dumps(res, ensure_ascii=False))
PY
)
build "$PAGES" "$ABS"
echo "PDF gerado: $OUT"
