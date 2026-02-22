{
  config,
  lib,
  pkgs,
  ...
}:
let
  cfg = config.services.demo-it;
  daemonExecutable = lib.getExe' cfg.package "demo-itd";
  environmentEntries = lib.mapAttrsToList (name: value: "${name}=${value}") cfg.environment;
  daemonCommand = lib.escapeShellArgs (
    [
      daemonExecutable
      "--socket"
      cfg.socketPath
    ]
    ++ cfg.extraArgs
  );
in
{
  options.services.demo-it = {
    enable = lib.mkEnableOption "demo-it background daemon";

    package = lib.mkOption {
      type = lib.types.package;
      default = pkgs.callPackage ../package.nix { };
      defaultText = lib.literalExpression "pkgs.callPackage ./nix/package.nix { }";
      description = "Package that provides both demo-it and demo-itd binaries.";
    };

    socketPath = lib.mkOption {
      type = lib.types.str;
      default = "${config.home.homeDirectory}/.local/state/demo-it/demo-it.sock";
      example = "/home/alice/.local/state/demo-it/demo-it.sock";
      description = "Socket path used by the daemon and exported via DEMO_IT_SOCKET.";
    };

    exportSocketEnvironment = lib.mkOption {
      type = lib.types.bool;
      default = true;
      description = "Export DEMO_IT_SOCKET in the user session.";
    };

    environment = lib.mkOption {
      type = lib.types.attrsOf lib.types.str;
      default = { };
      example = {
        DEMO_IT_SOCKET = "/home/alice/.local/state/demo-it/demo-it.sock";
      };
      description = "Additional environment variables for the systemd user service.";
    };

    extraArgs = lib.mkOption {
      type = lib.types.listOf lib.types.str;
      default = [ ];
      description = "Extra CLI arguments passed to demo-itd.";
    };
  };

  config = lib.mkIf cfg.enable {
    home.packages = [ cfg.package ];

    home.sessionVariables = lib.mkIf cfg.exportSocketEnvironment {
      DEMO_IT_SOCKET = cfg.socketPath;
    };

    systemd.user.services.demo-itd = {
      Unit = {
        Description = "demo-it background daemon";
      };

      Service = {
        ExecStart = daemonCommand;
        Environment = environmentEntries;
        Restart = "on-failure";
        RestartSec = "60s";
      };

      Install = {
        WantedBy = [ "default.target" ];
      };
    };
  };
}
