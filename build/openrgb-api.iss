; OpenRGB API Windows installer. Build with scripts/build-windows-installer.ps1.
#ifndef AppVersion
  #define AppVersion "unversioned"
#endif

[Setup]
AppId={{A0D6406D-6D1A-4B4F-980F-03EA3CD7D992}}
AppName=OpenRGB API
AppVersion={#AppVersion}
AppPublisher=Lightside Found (sb0-0dev)
DefaultDirName={autopf}\OpenRGB API
DefaultGroupName=OpenRGB API
OutputDir=..\release
OutputBaseFilename=OpenRGB-API-Setup-{#AppVersion}
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
PrivilegesRequired=admin
SetupLogging=yes
ArchitecturesInstallIn64BitMode=x64compatible
UninstallDisplayName=OpenRGB API
SetupIconFile=app-icon.ico
UninstallDisplayIcon={app}\app-icon.ico

[Tasks]
Name: "lan"; Description: "Allow mobile and other devices on your private network to connect"; GroupDescription: "Network access:"; Flags: unchecked
Name: "public"; Description: "Allow mobile and other devices on public networks to connect (not recommended)"; GroupDescription: "Network access:"; Flags: unchecked

[Dirs]
Name: "{commonappdata}\OpenRGB API"

[Files]
Source: "artifacts\windows\openrgb-api.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "app-icon.ico"; DestDir: "{app}"; Flags: ignoreversion
Source: "config-local.toml"; DestDir: "{commonappdata}\OpenRGB API"; DestName: "config.toml"; Flags: onlyifdoesntexist; Tasks: not lan
Source: "config-lan.toml"; DestDir: "{commonappdata}\OpenRGB API"; DestName: "config.toml"; Flags: onlyifdoesntexist; Tasks: lan

[Icons]
Name: "{group}\OpenRGB API"; Filename: "{sys}\rundll32.exe"; Parameters: "url.dll,FileProtocolHandler http://127.0.0.1:8743"; IconFilename: "{app}\app-icon.ico"

[Run]
Filename: "{app}\openrgb-api.exe"; Parameters: "-config ""{commonappdata}\OpenRGB API\config.toml"" install"; StatusMsg: "Registering the OpenRGB API service..."; Flags: runhidden waituntilterminated
Filename: "{app}\openrgb-api.exe"; Parameters: "-config ""{commonappdata}\OpenRGB API\config.toml"" start"; StatusMsg: "Starting the OpenRGB API service..."; Flags: runhidden waituntilterminated
Filename: "{sys}\netsh.exe"; Parameters: "advfirewall firewall add rule name=""OpenRGB API (Private)"" dir=in action=allow program=""{app}\openrgb-api.exe"" enable=yes profile=private protocol=TCP localport=8742"; Tasks: lan; StatusMsg: "Adding the private-network firewall rule..."; Flags: runhidden waituntilterminated
Filename: "{sys}\netsh.exe"; Parameters: "advfirewall firewall add rule name=""OpenRGB API (Public)"" dir=in action=allow program=""{app}\openrgb-api.exe"" enable=yes profile=public protocol=TCP localport=8742"; Tasks: public; StatusMsg: "Adding the public-network firewall rule..."; Flags: runhidden waituntilterminated
Filename: "{sys}\rundll32.exe"; Parameters: "url.dll,FileProtocolHandler http://127.0.0.1:8743"; Description: "Open the OpenRGB API control panel"; Flags: postinstall skipifsilent

[UninstallRun]
Filename: "{app}\openrgb-api.exe"; Parameters: "-config ""{commonappdata}\OpenRGB API\config.toml"" uninstall"; Flags: runhidden waituntilterminated; RunOnceId: "RemoveOpenRGBAPIService"
Filename: "{sys}\netsh.exe"; Parameters: "advfirewall firewall delete rule name=""OpenRGB API (Private)"""; Flags: runhidden waituntilterminated; RunOnceId: "RemoveOpenRGBAPIFirewallRule"
Filename: "{sys}\netsh.exe"; Parameters: "advfirewall firewall delete rule name=""OpenRGB API (Public)"""; Flags: runhidden waituntilterminated; RunOnceId: "RemoveOpenRGBAPIPublicFirewallRule"

[Code]
procedure RemovePreviousService();
var
  ResultCode: Integer;
  ExePath: String;
  ConfigPath: String;
begin
  ExePath := ExpandConstant('{app}\openrgb-api.exe');
  ConfigPath := ExpandConstant('{commonappdata}\OpenRGB API\config.toml');
  if FileExists(ExePath) and FileExists(ConfigPath) then begin
    Exec(ExePath, '-config "' + ConfigPath + '" stop', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
    Exec(ExePath, '-config "' + ConfigPath + '" uninstall', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  end;
  Exec(ExpandConstant('{sys}\netsh.exe'), 'advfirewall firewall delete rule name="OpenRGB API (Private)"', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  Exec(ExpandConstant('{sys}\netsh.exe'), 'advfirewall firewall delete rule name="OpenRGB API (Public)"', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep = ssInstall then
    RemovePreviousService();
end;
