{ inputs, lib, pkgs, ... }:

let
  nodejs = pkgs.nodejs_26;
  pgconsole = pkgs.callPackage ./devenv/pgconsole { inherit nodejs; };
  postgresql = inputs.nixpkgs-postgres.legacyPackages.${pkgs.stdenv.hostPlatform.system}.postgresql_18;
  postgisExtensions = extensions: [ extensions.postgis ];
in
{
  packages = [
    (inputs.go-overlay.lib.mkGoBin pkgs).versions."1.27.4"
    pkgs.jq
  ];

  # Plain JavaScript; no frontend build is needed.
  languages.javascript = {
    enable = true;
    package = nodejs;
    lsp.enable = false;
  };

  env.GOTOOLCHAIN = "local";

  profiles.ci.module = { config, ... }: {
    stdenv = pkgs.stdenvNoCC;
    env.CGO_ENABLED = "0";
    languages.javascript.package = pkgs.nodejs-slim_26;
    packages = [ (lib.getBin (postgresql.withPackages postgisExtensions)) ];
    services.postgres.enable = false;
    processes = lib.mkForce { };
    outputs.ci-image = pkgs.callPackage ./devenv/ci-image.nix { inherit config; };
  };

  services.postgres = {
    enable = true;
    package = postgresql;
    extensions = postgisExtensions;
    port = 5434;
    initdbArgs = [ "--locale=C" "--encoding=UTF8" "--auth-local=peer" "--auth-host=reject" ];
    settings.unix_socket_permissions = "0700";
    initialDatabases = [{ name = "cats"; }];
  };

  processes.app = {
    exec = "./scripts/run.sh";
    after = [ "devenv:processes:postgres@ready" ];
  };

  processes.pgconsole = {
    after = [ "devenv:processes:postgres@ready" ];
    exec = ''
      PGCONSOLE_CONFIG="$(
        jq -nr --arg host "$DEVENV_RUNTIME/postgres" --arg user "$(id -un)" \
          --arg port "$PGPORT" '[
            "[[connections]]",
            "id = \"cats\"",
            "name = \"Cats of Asia local\"",
            "database = \"cats\"",
            "host = \($host | @json)",
            "port = \($port | tonumber)",
            "username = \($user | @json)",
            "ssl_mode = \"disable\""
          ] | join("\n")'
      )"
      export PGCONSOLE_CONFIG
      unset PGHOST PGPORT PGUSER PGDATABASE PGPASSWORD PGSSLMODE
      exec ${pgconsole}/bin/pgconsole --port 3001
    '';
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
