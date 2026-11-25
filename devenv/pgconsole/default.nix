{ fetchurl, importNpmLock, nodejs, runCommand }:

let
  version = "1.3.0";
  archive = fetchurl {
    url = "https://registry.npmjs.org/@pgplex/pgconsole/-/pgconsole-${version}.tgz";
    hash = "sha512-xADGL3GYvp/RKczMqM+9oIEu07t6DjFOi/toOXfhiieDvpsbdMUCJlQ06jrJmcwnOGraMgLQo1UB/Nt5n6arIA==";
  };
  deps = importNpmLock.buildNodeModules {
    npmRoot = ./.;
    inherit nodejs;
    derivationArgs.npmRebuildFlags = [ "--ignore-scripts" ];
  };
in
runCommand "pgconsole-${version}" { } ''
  mkdir -p "$out/lib/pgconsole" "$out/bin"
  tar -xzf ${archive} --strip-components=1 -C "$out/lib/pgconsole"
  ln -s ${deps}/node_modules "$out/lib/pgconsole/node_modules"
  substituteInPlace "$out/lib/pgconsole/dist/server.mjs" \
    --replace-fail 'app.listen(port, () => {' 'app.listen(Number(port), "127.0.0.1", () => {' \
    --replace-fail 'app.use(express2.json());' 'app.use((req, res, next) => {
      const host = req.headers.host;
      if (!["127.0.0.1:3001", "localhost:3001"].includes(host) ||
          (req.headers.origin && req.headers.origin !== "http://" + host)) {
        return res.sendStatus(403);
      }
      next();
    });
    app.use(express2.json());'
  cat > "$out/bin/pgconsole" <<EOF
  #!$SHELL
  exec "${nodejs}/bin/node" "$out/lib/pgconsole/dist/server.mjs" "\$@"
  EOF
  chmod +x "$out/bin/pgconsole"
''
