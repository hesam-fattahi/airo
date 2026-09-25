# Contributing to AIRO

Thank you for your interest in contributing to AIRO! Contributions from developers of all levels are welcome and highly appreciated.

---

## 1. Code of Conduct

We are committed to creating a welcoming and inclusive environment for everyone. Please be respectful, professional, and collaborative when engaging in discussions, reviewing code, and contributing to the project. Any form of harassment or discrimination will not be tolerated.

---

## 2. Getting Started & Finding Issues

Before writing code, please check our open issues or create a new one:

1. **Browse Existing Issues:** Search open issues to see if your bug or feature request is already tracked.
2. **Issue Labels:**
   * `good-first-issue`: Great entry points for first-time contributors.
   * `help-wanted`: High-priority tasks looking for community assistance.
   * `bug`: Problems or unexpected behavior in AIRO.
   * `enhancement`: Feature requests and architecture improvements.
   * `documentation`: Improvements to `docs/` or the `README.md`.
3. **Assigning Issues:**
   * To prevent duplicate effort, **comment on the issue** requesting to be assigned before starting work (e.g., `"I would like to work on this issue."`).
   * A maintainer will assign the issue to you. Please wait for assignment before submitting a Pull Request.

---

## 3. Development Workflow

### Step 1: Fork and Clone

Fork the repository on GitHub, then clone your fork locally:

```bash
git clone https://github.com/hesam-fattahi/airo.git
cd airo
git remote add upstream https://github.com/hesam-fattahi/airo.git
```

### Step 2: Create a Feature Branch

Name your branch using standard prefix conventions:

```bash
git checkout -b <type>/<short-description>
```

**Allowed Types:**
* `feat/`: New features or CRD enhancements (e.g., `feat/my-cool-feature`)
* `fix/`: Bug fixes (e.g., `fix/endpointslice-reconciliation`)
* `docs/`: Documentation additions or fixes (e.g., `docs/sre-model`)
* `refactor/`: Code changes that neither fix a bug nor add a feature
* `test/`: Adding or updating unit/integration tests
* `chore/`: Build steps, CI workflow, or dependency updates

### Step 3: Local Verification

Run the local CI target to execute all code generators, CRD manifest checks, linters, and unit tests in a single command:

```bash
make ci
```

Ensure `make ci` passes with zero errors before pushing your code.

### Step 4: Submit a Pull Request

1. Push your branch to your fork:
   ```bash
   git push origin <type>/<short-description>
   ```
2. Open a Pull Request against the `main` branch of the official repository.
3. Reference the assigned issue in your PR description (e.g., `Closes #42` or `Fixes #42`).
4. Ensure all automated GitHub Actions CI checks pass.