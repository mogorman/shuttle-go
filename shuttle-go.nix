{
  lib,
  buildGoModule,
  fetchFromGitHub,
  dbus,
  libinput,
  xinput,
  procps,
  bash,
  makeWrapper,
}:

let
  runtimeDeps = [
    dbus
    libinput
    xinput
    procps
    bash
  ];
in
buildGoModule rec {
  pname = "shuttle-go";
  version = "1.0.14";

  src = ./.;

  subPackages = [];

  nativeBuildInputs = [ makeWrapper ] ++ runtimeDeps;

  vendorHash = "sha256-vwW+do+suS7gT0CkTEGdnIWlzWGJPZHhxEGgNGjIwS0=";
  doCheck = false;

  # Stamp the semantic version into the binary so `shuttle-go --version`
  # reports it. The Nix source has no .git, so we read the committed VERSION
  # file (the single source of truth, also used by build.sh).
  ldflags = [ "-X main.version=${lib.removeSuffix "\n" (builtins.readFile ./VERSION)}" ];

  postInstall = ''
    wrapProgram $out/bin/shuttle-go \
      --prefix PATH ":" ${lib.makeBinPath runtimeDeps}
  '';

  meta = with lib; {
    description = "Contour Design Shuttle Pro V2 drivers for Linux with modifiers";
    homepage = "https://github.com/abourget/shuttle-go";
    license = licenses.mit;
    maintainers = [];
    platforms = platforms.linux;
  };
}
