{
  lib,
  buildGoModule,
}:

buildGoModule {
  pname = "runner";
  version = "0.1.0";

  src = lib.fileset.toSource {
    root = ../.;
    fileset = lib.fileset.unions [
      ../go.mod
      ../go.sum
      ../cmd
      ../internal
    ];
  };

  vendorHash = "sha256-cTu/XpzKVgpHqUJ22ICtv6qGTd0Jmh9JXwM7BAg6E/E=";
  subPackages = [ "cmd/runner" ];
  env.CGO_ENABLED = "0";
  ldflags = [
    "-s"
    "-w"
  ];

  checkPhase = ''
    runHook preCheck
    go test -timeout 60s ./...
    runHook postCheck
  '';

  meta = {
    description = "Rerun terminal commands on file changes or named pokes";
    homepage = "https://git.3rr.dev/arch-err/runner";
    mainProgram = "runner";
    platforms = lib.platforms.linux ++ lib.platforms.darwin;
  };
}
