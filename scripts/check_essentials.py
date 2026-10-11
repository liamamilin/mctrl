"""Validate the structural core selection against frozen CLE registry definitions.

This checks coverage and dependency consistency, not frequency or efficacy.
"""
from collections import Counter
from pathlib import Path
import json
import re
import sys
from update_registry import parse_model_cards, field

ROOT = Path(__file__).resolve().parents[1]
audit = (ROOT / 'docs/essentials/SELECTION.md').read_text()
core = (ROOT / 'docs/essentials/index.md').read_text()
# This selection belongs to frozen v0.5, not later additions to the full book.
registry = {}
for model in ['M1', 'M2', 'M3', 'M4']:
    source = (ROOT / f'archive/full-v0.5-frozen/docs/models/{model}.md').read_text()
    for card in parse_model_cards(source):
        body = card['body']
        specialization = field(body, '类型')
        registry[card['id']] = {
            'model': model, 'components': field(body, '可复用组成'),
            'specializes': specialization[0] if specialization else None,
        }
errors = []

def present(ident):
    return re.search(r'(?<![A-Za-z0-9-])' + re.escape(ident) + r'(?![A-Za-z0-9-])', core)

def table(section):
    body = re.search(r'^## ' + re.escape(section) + r'\n(.*?)(?=^## |\Z)', audit, re.M | re.S)
    if not body:
        errors.append(f'Missing audit section: {section}')
        return {}
    rows = re.findall(r'^\| \[([^\]]+)\]\([^\n]+?\) \| (.+?) \|$', body[1], re.M)
    if len({ident for ident, _ in rows}) != len(rows):
        errors.append(f'Duplicate audit ID: {section}')
    return {ident: [v.strip() for v in rest.split(' | ')] for ident, rest in rows}

models = table('模型接口清单')
mechanisms = table('机制家族清单')
operators = table('算子清单')
protocols = table('协议清单')

if set(models) != set(registry):
    errors.append('Model audit must classify all and only 81 original interfaces')
for ident, fields in models.items():
    if len(fields) != 3 or ident not in registry:
        errors.append(f'Invalid model audit row: {ident}')
        continue
    if fields[0] != registry[ident]['model'] or fields[1] not in {'核心', '支撑', '扩展'} or not fields[2]:
        errors.append(f'Invalid model class or reason: {ident}')
retained = {ident for ident, fields in models.items() if len(fields) == 3 and fields[1] in {'核心', '支撑'}}
primary = {ident for ident, fields in models.items() if len(fields) == 3 and fields[1] == '核心'}
for ident in retained:
    item = registry[ident]
    dependencies = item['components'] + ([item['specializes']] if item['specializes'] else [])
    for dependency in dependencies:
        if dependency not in retained:
            errors.append(f'{ident}: missing original component/specialization parent {dependency}')
    if not present(ident):
        errors.append(f'{ident}: retained but absent from core reading page')
if not {'A-P14', 'A-P16'} <= retained:
    errors.append('Basic task completion must retain deliverable and acceptance binding support')

def check_family(rows, file, prefix, valid_tiers):
    source = ((ROOT / 'archive/full-v0.5-frozen' / file) if file.startswith('docs/engineering/') else (ROOT / file)).read_text()
    known = set(re.findall(r'^## (' + prefix + r'\d+)\.', source, re.M))
    if set(rows) != known:
        errors.append(f'{prefix} audit must cover the full original family')
    for ident, fields in rows.items():
        if len(fields) != 2 or fields[0] not in valid_tiers or not fields[1]:
            errors.append(f'Invalid audit class/reason: {ident}')
    return {ident for ident, fields in rows.items() if len(fields) == 2 and fields[0] == '核心'}

main_me = check_family(mechanisms, 'docs/MECHANISMS.md', 'ME', {'核心', '条件'})
main_op = check_family(operators, 'docs/engineering/OPERATORS.md', 'OP', {'核心', '扩展'})
main_pr = check_family(protocols, 'docs/engineering/PROTOCOLS.md', 'PR', {'核心', '扩展'})
op_source = (ROOT / 'docs/engineering/OPERATORS.md').read_text()
pattern_ids = set(re.findall(r'\*\*语言模式 (LP\d+)', op_source))
for ident in main_op:
    if 'LP' + ident[2:] not in pattern_ids:
        errors.append(f'{ident}: missing original principal language pattern')
pr_source = (ROOT / 'docs/engineering/PROTOCOLS.md').read_text()
for ident in main_pr:
    body = re.search(r'^## ' + ident + r'\. (.*?)(?=^## PR|\Z)', pr_source, re.M | re.S)[1]
    usage = re.search(r'^\*\*使用组件：\*\* (.+)$', body, re.M)[1]
    missing = set(re.findall(r'\bOP\d+\b', usage)) - main_op
    if missing:
        errors.append(f'{ident}: full retained protocol requires non-core operators {sorted(missing)}')
for ident in main_me | main_op | main_pr:
    if not present(ident):
        errors.append(f'{ident}: core selection absent from reading page')

print(json.dumps({
    'model_candidates': len(models), 'primary_interfaces': len(primary),
    'support_interfaces': len(retained - primary), 'retained_interfaces': len(retained),
    'retained_by_model': dict(Counter(registry[i]['model'] for i in retained)),
    'mechanism_candidates': len(mechanisms), 'core_mechanisms': len(main_me),
    'operator_candidates': len(operators), 'core_operators_and_principal_patterns': len(main_op),
    'protocol_candidates': len(protocols), 'fully_retained_protocols': sorted(main_pr),
    'errors': errors,
}, ensure_ascii=False, indent=2))
sys.exit(bool(errors))
