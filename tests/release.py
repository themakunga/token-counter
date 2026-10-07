#!/usr/bin/env python3
"""Run the real release workflow's Git steps against a disposable local remote."""
import os
from pathlib import Path
import subprocess
import tempfile
import textwrap


workflow = (Path(__file__).resolve().parents[1] / '.github/workflows/release.yml').read_text()


def script(name):
    block = workflow.split(f'      - name: {name}\n', 1)[1].split('\n      - ', 1)[0]
    return textwrap.dedent(block.split('        run: |\n', 1)[1])


with tempfile.TemporaryDirectory() as directory:
    root = Path(directory)
    repo, remote, bin_dir = root / 'repo', root / 'remote.git', root / 'bin'
    bin_dir.mkdir()
    gh = bin_dir / 'gh'
    gh.write_text('#!/bin/sh\nprintf "v0.3.1\\n"\n')
    gh.chmod(0o755)
    subprocess.run(['git', 'init', '--bare', str(remote)], check=True, capture_output=True)
    subprocess.run(['git', 'init', '-b', 'main', str(repo)], check=True, capture_output=True)

    def git(*args):
        return subprocess.check_output(['git', *args], cwd=repo, text=True, stderr=subprocess.DEVNULL).strip()

    git('config', 'user.name', 'Release test')
    git('config', 'user.email', 'test@example.invalid')
    (repo / 'VERSION').write_text('0.3.1\n')
    git('add', 'VERSION')
    git('commit', '-m', 'initial version')
    git('remote', 'add', 'origin', str(remote))
    env = dict(os.environ, PATH=f'{bin_dir}:{os.environ["PATH"]}', GITHUB_REF_NAME='release/v0.3.2', GITHUB_SHA=git('rev-parse', 'HEAD'), GITHUB_REPOSITORY='test/token-counter', GITHUB_OUTPUT=str(root / 'output'), VERSION='v0.3.2')

    def run(name, success=True):
        result = subprocess.run(['bash', '-e', '-o', 'pipefail', '-c', script(name)], cwd=repo, env=env, capture_output=True, text=True)
        assert (result.returncode == 0) == success, result.stdout + result.stderr

    run('Prepare release version')
    assert (repo / 'VERSION').read_text() == '0.3.2\n'
    assert not git('tag')  # Validation must happen before publishing tags.
    run('Publish immutable version tag')
    run('Advance stable channel after publishing')
    released = git('rev-parse', 'v0.3.2')
    assert git('ls-remote', 'origin', 'refs/tags/stable').split()[0] == released
    run('Prepare release version')  # A retry keeps the immutable release commit.
    assert git('rev-parse', 'HEAD') == released
    env['GITHUB_REF_NAME'] = 'release/v0.3.0'
    run('Prepare release version', success=False)
    env['GITHUB_REF_NAME'] = 'release/v0.3.3-rc.1'
    run('Prepare release version', success=False)
    assert git('ls-remote', 'origin', 'refs/tags/stable').split()[0] == released

print('Release version, immutable tag, stable channel and retries: OK')
