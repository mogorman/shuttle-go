{
  lib,
  buildGoModule,
  fetchFromGitHub,
  ydotool,
  dbus,
  libinput,
  xinput,
  procps,
  bash,
  makeWrapper,
}:

let
  runtimeDeps = [
    ydotool
    dbus
    libinput
    xinput
    procps
    bash
  ];
in
buildGoModule rec {
  pname = "shuttle-go";
  version = "0.9";

  src = ./.;

  subPackages = [];

  nativeBuildInputs = [ makeWrapper ] ++ runtimeDeps;

  vendorHash = "sha256-vwW+do+suS7gT0CkTEGdnIWlzWGJPZHhxEGgNGjIwS0=";
  doCheck = false;

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
