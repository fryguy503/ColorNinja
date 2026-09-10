"""Verify extracted archive integrity and execute its CLI and desktop service."""
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import tarfile
import tempfile
import time
import urllib.request

ROOT = Path(__file__).resolve().parent.parent


def sha(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def main():
    archive = Path(sys.argv[1]).resolve()
    assert sha(archive) == Path(str(archive) + '.sha256').read_text().split()[0]
    with tempfile.TemporaryDirectory(prefix='colorninja-verify-') as temp:
        root = Path(temp)
        extracted = root / 'extracted'
        extracted.mkdir()
        if archive.suffix == '.zip':
            subprocess.run(['ditto', '-x', '-k', str(archive), str(extracted)], check=True)
        else:
            with tarfile.open(archive) as bundle:
                bundle.extractall(extracted, filter='data')
        packages = list(extracted.iterdir())
        assert len(packages) == 1 and packages[0].is_dir()
        package = packages[0]
        listed = set()
        for line in (package / 'SHA256SUMS.txt').read_text().splitlines():
            expected, relative = line.split('  ', 1)
            file = (package / relative).resolve()
            assert file.is_relative_to(package) and file not in listed
            assert sha(file) == expected, relative
            listed.add(file)
        actual = {p.resolve() for p in package.rglob('*') if p.is_file() and p != package / 'SHA256SUMS.txt'}
        assert listed == actual, 'Unchecked or missing package files'
        info = json.loads((package / 'BUILD-INFO.json').read_text())
        gui, cli = package / info['gui'], package / info['cli']
        assert not info['sourceDirty'] and sha(gui) == info['guiSHA256'] and sha(cli) == info['cliSHA256']
        assert sha(package / 'SOURCE-MANIFEST.json') == info['sourceManifestSHA256']
        assert os.access(gui, os.X_OK) and os.access(cli, os.X_OK)
        image = package / 'internal/studio/assets/color-pop-poppy.png'
        library = ROOT / 'internal/engine/testdata/library.json'
        for mode in ('standard', 'guided', 'stack', 'color-pop'):
            args = [str(cli), str(image), '-o', str(root / f'{mode}.png'), '--colors', '4', '--palette-json', str(root / f'{mode}.json'), '--quiet']
            if mode != 'standard':
                args += ['--hueforge-library', str(library), '--hueforge-max-depth', '1.44']
            if mode in ('stack', 'color-pop'):
                args += ['--hueforge-stack', '--hueforge-project', str(root / f'{mode}.hfp')]
            if mode == 'color-pop':
                args += ['--color-pop', '--hueforge-max-runs', '8']
            subprocess.run(args, check=True, timeout=120)
            assert (root / f'{mode}.png').stat().st_size > 0
            if mode in ('stack', 'color-pop'):
                plan = json.loads((root / f'{mode}.json').read_text())['result']['stack']
                assert plan['uniqueFilaments'] <= 4 and plan['plannedDepth'] <= 1.440001
                assert json.loads((root / f'{mode}.hfp').read_text())['luminance_method'] == 6
        with (root / 'service.log').open('w') as log:
            process = subprocess.Popen([str(gui), '-dev-server', '127.0.0.1:48934', '-config-dir', str(root / 'config')], stdout=log, stderr=log)
            try:
                for _ in range(100):
                    assert process.poll() is None, (root / 'service.log').read_text()
                    try:
                        with urllib.request.urlopen('http://127.0.0.1:48934', timeout=1) as response:
                            assert b'<html' in response.read().lower()
                        break
                    except (OSError, TimeoutError):
                        time.sleep(.1)
                else:
                    raise AssertionError('Packaged desktop service did not start')
            finally:
                process.terminate()
                process.wait(timeout=15)
        report = dict(archive=archive.name, sha256=sha(archive), platform=info['platform'], sourceCommit=info['sourceCommit'], checkedFiles=len(listed), cliModes=4, desktopService=True,
                      nativeWindow='not automated', signing=info['signing'])
        destination = ROOT / 'artifacts/native-verification.json'
        destination.parent.mkdir(exist_ok=True)
        destination.write_text(json.dumps(report, indent=2) + '\n')
        print(json.dumps(report))


if __name__ == '__main__':
    main()
