{ config, pkgs }:

pkgs.dockerTools.buildLayeredImage {
  name = "cats-of-asia-ci";
  tag = "local";
  contents = [
    config.devenv.profile
    pkgs.bash
    pkgs.coreutils
    pkgs.cacert
    pkgs.dockerTools.usrBinEnv
  ];

  extraCommands = ''
    mkdir -p etc tmp workspace
    chmod 1777 tmp
    printf '%s\n' 'ci:x:1000:1000:CI:/tmp:/bin/bash' > etc/passwd
    printf '%s\n' 'ci:x:1000:' > etc/group
  '';

  config = {
    User = "1000:1000";
    WorkingDir = "/workspace";
    Cmd = [ "./scripts/run-tests.sh" ];
    Env = [
      "PATH=/bin"
      "HOME=/tmp"
      "SSL_CERT_FILE=${pkgs.cacert}/etc/ssl/certs/ca-bundle.crt"
      "CGO_ENABLED=${config.env.CGO_ENABLED}"
      "GOTOOLCHAIN=${config.env.GOTOOLCHAIN}"
    ];
    Labels = {
      "org.opencontainers.image.source" = "https://github.com/haikoschol/cats-of-asia-reloaded";
      "io.github.haikoschol.cats-of-asia-reloaded.ci-fingerprint" = config.env.COA_CI_FINGERPRINT;
    };
  };
}
