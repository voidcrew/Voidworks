; Windows installer. Compile through BUILD-INSTALLER.ps1, which supplies these:
;   Version         full version, such as 0.5.37 or 0.5.37-beta.1
;   NumericVersion  file version, such as 0.5.37.0
;   Package         folder holding the extracted release package
;   Output          folder for the finished setup program
;
; Installs for the current user only. The in-app updater renames files inside
; the installation folder, so that folder must stay writable without elevation
; (this rules out Program Files) and must hold the package's files directly.

[Setup]
; Keep this ID: it is how a later setup finds and upgrades an earlier install.
AppId={{6F0C9E2A-7B41-4D57-9A3E-2C1D8B5F4E60}
AppName=Voidworks
AppVerName=Voidworks {#Version}
; AppVersion is left out on purpose. The updater replaces the program without
; running setup, so a version stored for Windows' app list would go stale.
UninstallDisplayName=Voidworks
UninstallDisplayIcon={app}\Voidworks.exe
AppPublisher=Voidcrew
AppPublisherURL=https://github.com/voidcrew/Voidworks
AppSupportURL=https://github.com/voidcrew/Voidworks/issues
AppUpdatesURL=https://github.com/voidcrew/Voidworks/releases
VersionInfoVersion={#NumericVersion}
VersionInfoProductName=Voidworks
VersionInfoProductTextVersion={#Version}
VersionInfoDescription=Voidworks Setup
PrivilegesRequired=lowest
DefaultDirName={autopf}\Voidworks
DefaultGroupName=Voidworks
DisableProgramGroupPage=yes
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
MinVersion=10.0
; The checks in [Code] make the user close Voidworks, so unsaved work is never
; at risk from Setup closing it for them.
CloseApplications=no
SetupIconFile=..\internal\rsc\icon.ico
WizardStyle=modern
Compression=lzma2
SolidCompression=yes
OutputDir={#Output}
OutputBaseFilename=Voidworks-{#Version}-windows-x64-setup

[Tasks]
Name: desktopicon; Description: "Create a desktop shortcut"; Flags: unchecked

[Files]
; Exactly the files the updater manages. See packageFiles in internal/app/selfupdate.
Source: "{#Package}\Voidworks.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#Package}\shipcheck.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#Package}\LICENSE"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#Package}\START-HERE.txt"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#Package}\BUILD-INFO.json"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#Package}\SHA256SUMS.txt"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#Package}\Source.zip"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{autoprograms}\Voidworks"; Filename: "{app}\Voidworks.exe"
Name: "{autodesktop}\Voidworks"; Filename: "{app}\Voidworks.exe"; Tasks: desktopicon

[Run]
Filename: "{app}\Voidworks.exe"; Description: "Open Voidworks"; Flags: nowait postinstall skipifsilent

[UninstallDelete]
; A downloaded update waiting for a restart. Settings and backups live in the
; user's profile, outside this folder, and are kept.
Type: filesandordirs; Name: "{app}\.voidworks-update-*"

[Code]
function EditorIsRunning(const Executable: String): Boolean;
var
  Stream: TFileStream;
begin
  Result := False;
  if not FileExists(Executable) then
    Exit;
  try
    Stream := TFileStream.Create(Executable, fmOpenReadWrite or fmShareExclusive);
    Stream.Free;
  except
    Result := True;
  end;
end;

function WaitForEditorToClose(const Executable: String): Boolean;
begin
  Result := True;
  while Result and EditorIsRunning(Executable) do
    Result := SuppressibleMsgBox('Voidworks is running. Save your work and close it, then choose Retry.',
      mbError, MB_RETRYCANCEL, IDCANCEL) = IDRETRY;
end;

function PrepareToInstall(var NeedsRestart: Boolean): String;
begin
  Result := '';
  if not WaitForEditorToClose(ExpandConstant('{app}\Voidworks.exe')) then
    Result := 'Voidworks is still running. Close it and run this setup again.';
end;

function InitializeUninstall(): Boolean;
begin
  Result := WaitForEditorToClose(ExpandConstant('{app}\Voidworks.exe'));
end;
