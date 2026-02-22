{ pkgs, ... }:
{
  packages = with pkgs; [
    treefmt
    nixfmt
    gopls
    golangci-lint
    delve
    gofumpt
    gotools
    air
    stylua
    lua54Packages.luacheck
  ];

  languages.go.enable = true;
  languages.lua.enable = true;

  process.manager.implementation = "hivemind";

  process.manager.before = ''
    echo ""
    echo "demo-it daemon process"
    echo "  socket: auto (use --socket to override)"
    echo ""
    echo "Neovim local plugin testing"
    echo "  :set rtp+=$DEVENV_ROOT"
    echo "  :lua require(\"demo-it\").setup()"
    echo "  :DemoItReload"
    echo "  :DemoItStart"
    echo "  :DemoItStatus"
    echo "  :DemoItNext"
    echo ""
    echo "CLI testing"
    echo "  demo-it start"
    echo "  demo-it status"
    echo "  demo-it next"
    echo ""
  '';

  processes.demo-itd.exec = ''
    cd "$DEVENV_ROOT"
    air -c .air.toml
  '';

  scripts = {
    fmt.exec = "treefmt --config-file treefmt.toml --fail-on-change";
    fmt-fix.exec = "treefmt --config-file treefmt.toml";

    lint-go.exec = ''
      go vet ./...
      golangci-lint run
    '';

    lint-lua.exec = ''
      luacheck lua plugin
    '';

    lint.exec = ''
      lint-go
      lint-lua
    '';

    tests.exec = "go test ./...";

    build.exec = ''
      mkdir -p bin
      go build -o bin/demo-it ./cmd/demo-it
      go build -o bin/demo-itd ./cmd/demo-itd
    '';

    ci.exec = ''
      set -euo pipefail
      fmt
      lint
      tests
      build
    '';
  };

  enterShell = ''
    export PATH="$DEVENV_ROOT/bin:$PATH"

    if [[ -n "''${DIRENV_IN_ENVRC:-}" || $- == *i* ]]; then
      echo "╔══════════════════════════════════════════════════════════╗"
      echo "║  demo-it                                                 ║"
      echo "╚══════════════════════════════════════════════════════════╝"
      echo ""
      echo "Go commands:"
      echo "  go run ./cmd/demo-itd   - start daemon"
      echo "  go run ./cmd/demo-it -- start|status|next|prev"
      echo ""
      echo "Process orchestration:"
      echo "  devenv up              - start demo-itd (auto-reload via air)"
      echo ""
      echo "Neovim plugin (local runtimepath):"
      echo "  :set rtp+=''${DEVENV_ROOT:-$PWD}"
      echo "  :lua require(\"demo-it\").setup()"
      echo "  :DemoItReload"
      echo ""
      echo "Tooling:"
      echo "  gopls | golangci-lint | delve | gofumpt"
      echo "  lua-language-server | stylua | luacheck"
      echo ""
      echo "CI commands:"
      echo "  fmt     - formatting check"
      echo "  fmt-fix - apply formatting"
      echo "  lint    - Go + Lua lint"
      echo "  tests"
      echo "  build"
      echo "  ci"
    fi
  '';
}
