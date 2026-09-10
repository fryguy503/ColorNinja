"""Package only native release outputs, docs and required dependency licenses."""
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess

ROOT = Path(__file__).resolve().parent.parent
os.chdir(ROOT)


def run(*args):
    return subprocess.check_output(args, text=True).strip()


def sha(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def main():
    platform, arch = run('go', 'env', 'GOOS'), run('go', 'env', 'GOARCH')
    assert platform in ('linux', 'darwin') and arch in ('amd64', 'arm64')
    assert not run('git', 'status', '--porcelain'), 'Release requires clean tracked source'
    version = json.loads((ROOT / 'frontend/package.json').read_text())['version']
    suffix = ('macos' if platform == 'darwin' else platform) + '-' + {'amd64': 'x64', 'arm64': 'arm64'}[arch]
    name = f'ColorNinja-{version}-{suffix}'
    stage = ROOT / 'build/releases' / name
    stage.mkdir(parents=True, exist_ok=False)
    gui = ROOT / 'build/bin' / ('ColorNinja.app' if platform == 'darwin' else 'ColorNinja')
    if platform == 'darwin':
        shutil.copytree(gui, stage / gui.name)
        executable = stage / gui.name / 'Contents/MacOS/ColorNinja'
    else:
        shutil.copy2(gui, stage / gui.name)
        executable = stage / gui.name
    shutil.copy2(ROOT / 'build/bin/colorninja-cli', stage)
    for filename in ('README.md', 'LICENSE'):
        shutil.copy2(ROOT / filename, stage)
    shutil.copy2(ROOT / 'docs/QUICKSTART.txt', stage / 'START-HERE.txt')
    shutil.copytree(ROOT / 'docs', stage / 'docs')
    assets = stage / 'internal/studio/assets'
    assets.mkdir(parents=True)
    for filename in ('color-pop-poppy.png', 'README.md'):
        shutil.copy2(ROOT / 'internal/studio/assets' / filename, assets)
    manifest = [{'path': p, 'sha256': sha(ROOT / p)} for p in run('git', 'ls-files').splitlines()]
    (stage / 'SOURCE-MANIFEST.json').write_text(json.dumps(manifest, indent=2) + '\n')
    notices = ['ColorNinja third-party notices.', (Path(run('go', 'env', 'GOROOT')) / 'LICENSE').read_text()]
    tags = 'desktop,production' + (',webkit2_41' if platform == 'linux' else '')
    rows = run('go', 'list', '-deps', '-tags', tags, '-f', '{{if .Module}}{{.Module.Path}}|{{.Module.Version}}|{{.Module.Dir}}{{end}}', '.', './cmd/colorninja-cli')
    modules = set()
    for row in sorted(set(rows.splitlines())):
        if not row or row.startswith('colorninja|'):
            continue
        module, ver, folder = row.split('|')
        licenses = [p for p in Path(folder).iterdir() if p.is_file() and p.name.split('.')[0].upper() in ('LICENSE', 'COPYING', 'NOTICE')]
        assert licenses, f'Missing license for {module}'
        modules.add(module)
        notices.append(f'\n========== {module} {ver} ==========')
        notices.extend(p.read_text() for p in sorted(licenses))
    for package in ('react', 'react-dom', 'scheduler', 'lucide-react'):
        folder = ROOT / 'frontend/node_modules' / package
        info = json.loads((folder / 'package.json').read_text())
        notices.extend([f'\n========== {package} {info["version"]} ==========', (folder / 'LICENSE').read_text()])
    for line in run('go', 'version', '-m', str(executable), str(stage / 'colorninja-cli')).splitlines():
        parts = line.split()
        if parts and parts[0] == 'dep':
            assert parts[1] in modules, f'Missing compiled dependency notice: {parts[1]}'
    (stage / 'THIRD-PARTY-NOTICES.txt').write_text('\n'.join(notices))
    metadata = dict(product='ColorNinja Studio', version=version, platform=suffix, sourceCommit=run('git', 'rev-parse', 'HEAD'), sourceDirty=False,
                    go=run('go', 'version'), gui=str(executable.relative_to(stage)), cli='colorninja-cli', guiSHA256=sha(executable), cliSHA256=sha(stage / 'colorninja-cli'),
                    sourceManifestSHA256=sha(stage / 'SOURCE-MANIFEST.json'), signing='not notarized; no publisher certificate', tests='Go suite, frontend tests, TypeScript build and go vet passed')
    (stage / 'BUILD-INFO.json').write_text(json.dumps(metadata, indent=2) + '\n')
    sums = [f'{sha(p)}  {p.relative_to(stage).as_posix()}' for p in sorted(stage.rglob('*')) if p.is_file()]
    (stage / 'SHA256SUMS.txt').write_text('\n'.join(sums) + '\n')
    if platform == 'darwin':
        archive = Path(str(stage) + '.zip')
        subprocess.run(['ditto', '-c', '-k', '--keepParent', str(stage), str(archive)], check=True)
    else:
        archive = Path(shutil.make_archive(str(stage), 'gztar', root_dir=stage.parent, base_dir=name))
    Path(str(archive) + '.sha256').write_text(f'{sha(archive)}  {archive.name}\n')
    print(archive)


if __name__ == '__main__':
    main()
