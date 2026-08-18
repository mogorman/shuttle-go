{
  lib,
  buildGoModule,
  fetchFromGitHub,
  xdotool,
  ydotool,
  dbus,
  libinput,
}:

buildGoModule rec {
  pname = "shuttle-go";
  version = "0.9";

  src = ./.;

  subPackages = [];

  nativeBuildInputs = [
    xdotool
    ydotool
    dbus
    libinput
  ];

  vendorHash = "sha256-vwW+do+suS7gT0CkTEGdnIWlzWGJPZHhxEGgNGjIwS0=";
  doCheck = false;

  meta = with lib; {
    description = "Contour Design Shuttle Pro V2 drivers for Linux with modifiers";
    homepage = "https://github.com/abourget/shuttle-go";
    license = licenses.mit;
    maintainers = [];
    platforms = platforms.linux;
  };
}
