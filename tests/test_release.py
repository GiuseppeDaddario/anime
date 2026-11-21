import hashlib
import importlib.util
from pathlib import Path
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('formula_generator', ROOT / 'scripts/generate-formula.py')
formula = importlib.util.module_from_spec(spec)
spec.loader.exec_module(formula)

class ReleaseTests(unittest.TestCase):
    def test_formula_checksums_are_derived_from_both_actual_assets(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for arch in ('arm64', 'amd64'):
                (root / f'anime_v0.1.0_darwin_{arch}.tar.gz').write_bytes(arch.encode())
            generated = formula.generate('GiuseppeDaddario/anime', 'v0.1.0', root)
            for arch in ('arm64', 'amd64'):
                self.assertIn(hashlib.sha256(arch.encode()).hexdigest(), generated)
                self.assertIn(f'anime_v0.1.0_darwin_{arch}.tar.gz', generated)
            self.assertIn('depends_on "mpv"', generated)
            self.assertNotIn('python', generated)
            self.assertNotIn('playwright', generated)

    def test_missing_assets_and_injected_metadata_are_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            for repository, version in [('owner/repo;bad', 'v0.1.0'), ('owner/repo', 'bad"')]:
                with self.assertRaises(ValueError):
                    formula.generate(repository, version, Path(directory))
            with self.assertRaises(FileNotFoundError):
                formula.generate('owner/repo', 'v0.1.0', Path(directory))

if __name__ == '__main__': unittest.main()
