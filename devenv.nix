{ ... }:
{
  languages.go.enable = true;

  treefmt = {
    enable = true;
    config = {
      settings.excludes = [
        ".devenv/*"
        ".direnv/*"
        "result/*"
      ];
      programs.gofmt.enable = true;
      programs.nixfmt.enable = true;
    };
  };

  scripts = {
    fmt.exec = "treefmt --fail-on-change";
    lint.exec = "go vet ./...";
    tests.exec = "go test ./...";
    build.exec = ''
      go build ./cmd/demo-it
      go build ./cmd/demo-itd
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
    if [[ $- == *i* ]]; then
      echo "╔════════════════════════════════════════════════════════════════╗"
      echo "║  demo-it                                                     ║"
      echo "╚════════════════════════════════════════════════════════════════╝"
      echo ""
      echo "Go commands:"
      echo "  go run ./cmd/demo-itd   - start daemon"
      echo "  go run ./cmd/demo-it -- start|status|next|prev"
      echo ""
      echo "Neovim plugin (local runtimepath):"
      echo "  :set rtp+=${toString ./.}"
      echo "  :DemoItReload"
      echo ""
      echo "CI commands:"
      echo "  fmt"
      echo "  lint"
      echo "  tests"
      echo "  build"
      echo "  ci"
    fi
  '';
}
