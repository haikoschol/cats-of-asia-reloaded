{ inputs, pkgs, ... }:

{
  packages = [
    (inputs.go-overlay.lib.mkGoBin pkgs).versions."1.27.4"
    pkgs.jq
  ];

  # Plain JavaScript; Node.js is only needed for the existing tests.
  languages.javascript = {
    enable = true;
    package = pkgs.nodejs_24;
  };

  env.GOTOOLCHAIN = "local";

  services.postgres = {
    enable = true;
    package = inputs.nixpkgs-postgres.legacyPackages.${pkgs.stdenv.hostPlatform.system}.postgresql_18;
    extensions = extensions: [ extensions.postgis ];
    port = 5434;
    initdbArgs = [ "--locale=C" "--encoding=UTF8" "--auth-local=peer" "--auth-host=reject" ];
    settings.unix_socket_permissions = "0700";
    initialDatabases = [{ name = "cats"; }];
  };

  processes.app = {
    exec = "./scripts/run.sh";
    after = [ "devenv:processes:postgres@ready" ];
  };

  tasks = {
    "app:build".exec = "go build .";
    "app:test:unit".exec = "./scripts/run-tests.sh unit";
    "app:test:integration".exec = "./scripts/run-tests.sh integration";
    "app:test".exec = "./scripts/run-tests.sh";
    "app:release".exec = ''
      set -euo pipefail
      commit=$(git rev-parse HEAD)
      binary="coa-''${commit:0:7}-linux-amd64"
      mkdir -p dist
      CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o "dist/$binary" .
      (cd dist && ${pkgs.coreutils}/bin/sha256sum "$binary" > "$binary.sha256")
    '';
  };

  dotenv.disableHint = true;
}
