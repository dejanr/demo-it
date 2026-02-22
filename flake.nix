{
  description = "demo-it backend and Neovim client";

  inputs = {
    devenv-root = {
      url = "file+file:///dev/null";
      flake = false;
    };
    flake-parts.url = "github:hercules-ci/flake-parts";
    flake-parts.inputs.nixpkgs-lib.follows = "nixpkgs";
    nixpkgs.url = "github:cachix/devenv-nixpkgs/rolling";
    devenv.url = "github:cachix/devenv";
    devenv.inputs.nixpkgs.follows = "nixpkgs";
    devenv.inputs.flake-parts.follows = "flake-parts";
  };

  outputs =
    inputs@{
      flake-parts,
      ...
    }:
    flake-parts.lib.mkFlake { inherit inputs; } {
      imports = [ inputs.devenv.flakeModule ];

      systems = [
        "x86_64-linux"
        "aarch64-linux"
      ];

      perSystem =
        { pkgs, config, ... }:
        {
          formatter = pkgs.writeShellApplication {
            name = "demo-it-treefmt";
            runtimeInputs = with pkgs; [
              treefmt
              nixfmt
              go
              stylua
            ];
            text = ''
              exec treefmt --config-file ${./treefmt.toml} "$@"
            '';
          };

          packages.default = config.devenv.shells.default.config.ci;

          devenv.shells.default = {
            name = "demo-it";
            imports = [ ./devenv.nix ];
          };
        };
    };
}
