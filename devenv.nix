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
    echo "  socket: $DEVENV_ROOT/.devenv/demo-itd.sock"
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
    mkdir -p "$DEVENV_ROOT/.devenv"
    export DEMO_IT_SOCKET="$DEVENV_ROOT/.devenv/demo-itd.sock"
    export DEMO_IT_REQUIRE_LOCAL_DAEMON=1
    air -c .air.toml -build.args_bin "--socket=$DEMO_IT_SOCKET"
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
    tests-e2e.exec = "go test -tags=e2e ./test/e2e";

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
    export DEMO_IT_SOCKET="$DEVENV_ROOT/.devenv/demo-itd.sock"
    export DEMO_IT_REQUIRE_LOCAL_DAEMON=1

    if [[ -n "''${DIRENV_IN_ENVRC:-}" || $- == *i* ]]; then
      echo "# demo-it"
      echo ""
      echo "devenv:"
      echo "  devenv up - start demo-it daemon (auto-reload)"
      echo "  fmt       - formatting check"
      echo "  fmt-fix   - apply formatting"
      echo "  lint      - Go + Lua lint"
      echo "  tests"
      echo "  build"
      echo "  ci"
      echo ""
      echo "nvim plugin (local runtimepath):"
      echo "  :set rtp+=''${DEVENV_ROOT:-$PWD}"
      echo "  :lua require(\"demo-it\").setup()"
      echo "  :DemoItReload"
      echo ""
    fi
  '';
}
