{ inputs, pkgs, ... }:

{
  packages = [ (inputs.go-overlay.lib.mkGoBin pkgs).versions."1.27.4" ];

  # Plain JavaScript; Node.js is only needed for the existing tests.
  languages.javascript = {
    enable = true;
    package = pkgs.nodejs_24;
  };

  env.GOTOOLCHAIN = "local";

  # The Go application and Docker Compose already read .env.
  dotenv.disableHint = true;
}
