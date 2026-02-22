---
name: nix-go-vendor-hash
description: "Update vendorHash in Nix Go package definitions (buildGoModule). Use when Go dependencies change and nix build fails with hash mismatch."
---

# Nix Go Vendor Hash Skill

Update `vendorHash` in Nix `buildGoModule` package definitions when Go dependencies change.

## Quick Reference

```bash
# Method 1: Dummy hash approach (recommended)
# 1. Set dummy hash in .nix file
vendorHash = "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=";

# 2. Build to get correct hash
nix build .#<package-name> 2>&1 | grep "got:"
# Output: got:    sha256-<correct-hash>

# 3. Update with correct hash

# Method 2: Local vendor approach
go mod vendor
nix-hash --type sha256 --sri vendor/
rm -rf vendor/
```

## When to Update vendorHash

Update `vendorHash` when:

- Adding/removing Go dependencies in `go.mod`
- Running `go mod tidy` changes `go.sum`
- Upgrading dependency versions
- Nix build fails with "hash mismatch" error

## Method 1: Dummy Hash (Recommended)

The fastest method - let Nix compute the hash for you.

### Step 1: Set Dummy Hash

Edit the Nix package file (e.g., `nix/packages/<name>.nix`):

```nix
buildGoModule {
  # ...
  vendorHash = "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=";
  # ...
}
```

### Step 2: Build and Extract Hash

```bash
# Stage changes first (Nix uses git)
git add .

# Build and capture the correct hash
nix build .#<package-name> 2>&1 | grep "got:"
```

Output will show:

```
got:    sha256-<actual-correct-hash>=
```

### Step 3: Update with Correct Hash

Replace the dummy hash with the computed hash:

```nix
vendorHash = "sha256-<actual-correct-hash>=";
```

### Step 4: Verify Build

```bash
nix build .#<package-name>
```

## Method 2: Local Vendor Directory

Alternative approach using local vendoring.

### Step 1: Vendor Dependencies

```bash
cd <go-module-dir>
nix develop --impure --command go mod vendor
```

### Step 2: Compute Hash

```bash
nix-hash --type sha256 --sri vendor/
```

### Step 3: Update Nix File

Use the output hash in your package:

```nix
vendorHash = "sha256-<computed-hash>=";
```

### Step 4: Clean Up

```bash
rm -rf vendor/
```

## Common Errors

### Hash Mismatch

```
error: hash mismatch in fixed-output derivation
  specified: sha256-AAAA...
  got:       sha256-BBBB...
```

**Solution**: Use the "got" hash in your Nix file.

### Vendor Modules.txt Mismatch

```
go.mod but not marked as explicit in vendor/modules.txt
```

**Solution**: Run `go mod tidy` then `go mod vendor` before computing hash.

### Git Tree is Dirty

```
warning: Git tree is dirty
```

**Solution**: Stage your changes with `git add .` before building.

## Example Workflow

Complete example updating vendorHash after adding a dependency:

```bash
# 1. Add dependency
cd backend/my-service
nix develop --impure --command go get github.com/some/package
nix develop --impure --command go mod tidy

# 2. Set dummy hash
# Edit nix/packages/my-service.nix:
#   vendorHash = "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=";

# 3. Stage and build
git add .
nix build .#my-service 2>&1 | grep "got:"
# got:    sha256-xYz123...=

# 4. Update with correct hash
# Edit nix/packages/my-service.nix:
#   vendorHash = "sha256-xYz123...=";

# 5. Verify
nix build .#my-service
```

## Tips

### vendorHash vs vendorSha256

- Use `vendorHash` (newer, preferred) with SRI format: `sha256-...=`
- `vendorSha256` is deprecated

### Null vendorHash

Setting `vendorHash = null;` means no vendoring (dependencies fetched at build time). Use sparingly - prefer explicit hashes for reproducibility.

### CI/CD Considerations

Always commit the updated hash together with `go.mod` and `go.sum` changes to ensure reproducible builds in CI.
