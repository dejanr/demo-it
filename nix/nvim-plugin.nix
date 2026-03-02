{ lib, vimUtils }:

let
  root = toString ../.;
  runtimeDirs = [
    "plugin"
    "lua"
    "after"
    "doc"
    "ftplugin"
    "syntax"
  ];

  toRelativePath =
    path:
    let
      pathStr = toString path;
    in
    if pathStr == root then "" else lib.removePrefix "${root}/" pathStr;

  isRuntimePath =
    rel: rel == "" || lib.any (dir: rel == dir || lib.hasPrefix "${dir}/" rel) runtimeDirs;

  src = lib.cleanSourceWith {
    src = ../.;
    filter = path: _: isRuntimePath (toRelativePath path);
  };
in
vimUtils.buildVimPlugin {
  pname = "demo-it.nvim";
  version = "0.0.0";

  inherit src;

  meta = {
    description = "Neovim plugin for demo-it";
    homepage = "https://github.com/dejanr/demo-it";
    license = lib.licenses.mit;
  };
}
