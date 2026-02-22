{ buildGoModule, lib }:

buildGoModule {
  pname = "demo-it";
  version = "0.0.0";

  src = lib.cleanSource ../.;

  subPackages = [
    "cmd/demo-it"
    "cmd/demo-itd"
  ];

  vendorHash = "sha256-g+yaVIx4jxpAQ/+WrGKxhVeliYx7nLQe/zsGpxV4Fn4=";

  meta = {
    description = "Transcript-driven demo CLI and background daemon";
    homepage = "https://github.com/dejanr/demo-it";
    license = lib.licenses.mit;
    mainProgram = "demo-it";
  };
}
