import re, sys, pathlib
bad = 0
for f in sorted(pathlib.Path('exec').glob('*.go')):
    lines = f.read_text().split('\n')
    for i, l in enumerate(lines):
        if l.startswith('func '):
            name = l[5:l.index('(')] if '(' in l else l
            prev = lines[i-1] if i else ''
            if not prev.startswith('//'):
                print(f"{f}:{i+1}: func {name} has NO doc comment directly above it (prev={prev.strip()[:50]!r})")
                bad += 1
    # and the inverse: a comment block separated from its func by a blank line
    for i, l in enumerate(lines[:-1]):
        if l == '' and i > 0 and lines[i-1].startswith('//') and lines[i+1].startswith('func '):
            print(f"{f}:{i+1}: doc block DETACHED from the func below it by a blank line")
            bad += 1
print("doc-attachment check:", "OK" if bad == 0 else f"{bad} problem(s)")
sys.exit(1 if bad else 0)
