"""Export the single Markdown source into complete Markdown/HTML reading copies."""
from pathlib import Path
from html import escape, unescape
from html.parser import HTMLParser
from urllib.parse import urlsplit, unquote
import argparse
import re
import markdown
import yaml

ROOT = Path(__file__).resolve().parents[1]
DOCS = ROOT / 'docs'

def nav_entries(items):
    for item in items:
        for label, value in item.items():
            if isinstance(value, list):
                yield from nav_entries(value)
            else:
                yield label, (DOCS / value).resolve()

class Headings(HTMLParser):
    def __init__(self, html):
        super().__init__()
        self.ids = []
        self.feed(html)
    def handle_starttag(self, tag, attrs):
        if re.fullmatch(r'h[1-6]', tag):
            self.ids.append(dict(attrs).get('id', ''))

def export(destination):
    destination.mkdir(parents=True, exist_ok=True)
    core = (DOCS / 'CORE_SPEC.md').read_text()
    version = re.search(r'Core Spec (v[\d.]+)', core).group(1)
    date = re.search(r'日期：(\d{4}-\d{2}-\d{2})', core).group(1)
    status = re.search(r'状态：\*\*([^*]+)\*\*', core).group(1)
    entries = list(nav_entries(yaml.safe_load((ROOT / 'mkdocs.yml').read_text())['nav']))
    credit_match = re.search(r'^\*\*作者：\*\* .+$', (DOCS / 'index.md').read_text(), re.MULTILINE)
    credit = credit_match.group(0) if credit_match else ''
    credit_html = markdown.markdown(credit)
    prefixes = {p: f'chapter-{i:02}' for i, (_, p) in enumerate(entries, 1)}
    def link(href, current):
        u = urlsplit(unescape(href))
        if u.scheme or u.netloc:
            return href
        target = (current.parent / unquote(u.path)).resolve() if u.path else current
        if target in prefixes:
            return '#' + prefixes[target] + ('-' + unquote(u.fragment) if u.fragment else '')
        return href

    html_sections, md_sections = [], []
    for number, (label, path) in enumerate(entries, 1):
        prefix = prefixes[path]
        text = path.read_text()
        if text.startswith('---\n'):
            text = text.split('---\n', 2)[-1]
        rendered = markdown.markdown(text, extensions=['tables', 'toc', 'fenced_code', 'attr_list', 'md_in_html'])
        heading_ids = iter(Headings(rendered).ids)
        rendered = re.sub(r'id="([^"]+)"', lambda m: f'id="{prefix}-{m[1]}"', rendered)
        rendered = re.sub(r'href="([^"]+)"', lambda m: 'href="' + escape(link(m[1], path), quote=True) + '"', rendered)
        rendered = rendered.replace('<table>', '<div class="table-wrap"><table>').replace('</table>', '</table></div>')
        html_sections.append(f'<section id="{prefix}"><div class="chapter-number">{number:02} / {len(entries)}</div>{rendered}</section>')

        md = re.sub(r'<a id="([^"]+)"></a>', lambda m: f'<a id="{prefix}-{m[1]}"></a>', text)
        md = re.sub(r'\[([^\]]+)\]\(([^)\s]+)\)', lambda m: '[' + m[1] + '](' + link(m[2], path) + ')', md)
        lines = []
        in_fence = False
        for line in md.splitlines():
            if line.startswith('```'):
                in_fence = not in_fence
            if not in_fence and re.match(r'^#{1,6} ', line):
                hid = next(heading_ids)
                lines.append(f'<a id="{prefix}-{hid}"></a>\n')
                line = re.sub(r'\s*\{#[^}]+\}\s*$', '', line)
                line = '#' + line if not line.startswith('###### ') else line
            lines.append(line)
        md_sections.append(f'<a id="{prefix}"></a>\n\n' + '\n'.join(lines))

    md_toc = '\n'.join(f'{i}. [{label}](#{prefixes[p]})' for i, (label, p) in enumerate(entries, 1))
    manuscript = f'# 认知语言工程｜完整体系 {version}\n\n{credit}\n\n{date} · {status}\n\n本稿由项目正文生成，包含理论架构、工程组件、使用案例、评估、来源和维护说明。修改以项目 docs 为准。\n\n## 目录\n\n' + md_toc + '\n\n---\n\n' + '\n\n---\n\n'.join(md_sections) + '\n'
    md_path = destination / f'CLE_{version}_完整体系.md'
    md_path.write_text(manuscript)
    toc = ''.join(f'<li><a href="#{prefixes[p]}"><span>{i:02}</span>{escape(label)}</a></li>' for i, (label, p) in enumerate(entries, 1))
    template = (ROOT / 'overrides/reading.html').read_text()
    template = template.replace('READER_CSS', (DOCS / 'assets/stylesheets/reader.css').read_text())
    template = template.replace('READER_JS', (DOCS / 'assets/javascripts/reader.js').read_text())
    html_path = destination / f'CLE_{version}_完整体系.html'
    template = template.replace('AUTHOR_CREDIT', credit_html)
    html_path.write_text(template.replace('VERSION', version).replace('DATE', date).replace('RELEASE_STATUS', escape(status)).replace('CHAPTER_COUNT', str(len(entries))).replace('TOC', toc).replace('CONTENT', '\n'.join(html_sections)))
    return md_path, html_path

if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--output-dir', type=Path, required=True)
    args = parser.parse_args()
    for path in export(args.output_dir):
        print(path)
