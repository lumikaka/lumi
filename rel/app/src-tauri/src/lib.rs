use base64::{engine::general_purpose::URL_SAFE_NO_PAD, Engine as _};
use std::fs::{self, OpenOptions};
use std::io::{self, Read, Write};
use std::net::{Ipv4Addr, SocketAddr, SocketAddrV4, TcpListener, TcpStream};
use std::path::{Path, PathBuf};
use std::process::{Child, Command, Stdio};
#[cfg(feature = "desktop-updater")]
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};
#[cfg(target_os = "windows")]
use std::{ffi::OsStr, os::windows::process::CommandExt};
use tauri::menu::{Menu, MenuItem, PredefinedMenuItem};
use tauri::tray::TrayIconBuilder;
use tauri::{AppHandle, Manager};
use tauri_plugin_clipboard_manager::ClipboardExt;
#[cfg(feature = "desktop-updater")]
use tauri_plugin_dialog::MessageDialogButtons;
use tauri_plugin_dialog::{DialogExt, MessageDialogKind};
#[cfg(feature = "desktop-updater")]
use tauri_plugin_updater::UpdaterExt;

const APP_NAME: &str = "Lumi";
const TRAY_ID: &str = "lumi-tray";
const STARTUP_TIMEOUT: Duration = Duration::from_secs(30);
const HEALTH_INTERVAL: Duration = Duration::from_millis(150);
const DESKTOP_ACCESS_TOKEN_ENV: &str = "LUMI_DESKTOP_ACCESS_TOKEN";
#[cfg(target_os = "windows")]
const BACKEND_BINARY_NAME: &str = "lumi_web.exe";
#[cfg(not(target_os = "windows"))]
const BACKEND_BINARY_NAME: &str = "lumi_web";
#[cfg(target_os = "windows")]
const DEBUG_BACKEND_DIRECTORY: &str = "backend-windows";
#[cfg(not(target_os = "windows"))]
const DEBUG_BACKEND_DIRECTORY: &str = "backend-darwin";
#[cfg(target_os = "windows")]
const CREATE_NO_WINDOW: u32 = 0x08000000;
#[cfg(target_os = "windows")]
const OPEN_TARGET_ENV: &str = "LUMI_OPEN_TARGET";
#[cfg(target_os = "windows")]
const OPEN_TARGET_SCRIPT: &str =
    "$target = $env:LUMI_OPEN_TARGET; Remove-Item Env:LUMI_OPEN_TARGET; Start-Process -FilePath $target";

pub fn run() {
    let builder = tauri::Builder::default()
        .plugin(tauri_plugin_single_instance::init(|app, _argv, _cwd| {
            if let Some(state) = app.try_state::<LauncherState>() {
                state.request_open();
            }
        }))
        .plugin(tauri_plugin_dialog::init())
        .plugin(tauri_plugin_clipboard_manager::init())
        .setup(|app| {
            let log_path = log_path(app.handle())?;
            ensure_parent_dir(&log_path)?;
            write_log(&log_path, "starting Lumi desktop launcher");

            let port = select_port()?;
            let base_url = app_url(port);
            let access_token = generate_access_token().map_err(io::Error::other)?;
            let access_url = desktop_access_url(&base_url, &access_token);
            let state = LauncherState::new(base_url, access_url, log_path.clone());
            app.manage(state);

            #[cfg(feature = "desktop-updater")]
            {
                app.handle()
                    .plugin(tauri_plugin_updater::Builder::new().build())?;
            }

            build_tray(app.handle())?;
            app.state::<LauncherState>().request_open();

            let handle = app.handle().clone();
            tauri::async_runtime::spawn_blocking(move || {
                launch_and_monitor_backend(handle, port, access_token, log_path);
            });

            #[cfg(feature = "desktop-updater")]
            check_for_updates(app.handle().clone(), UpdateCheckOrigin::Startup);

            Ok(())
        });

    let app = builder
        .build(tauri::generate_context!())
        .expect("error while building Lumi desktop launcher");

    app.run(|app_handle, event| match event {
        #[cfg(target_os = "macos")]
        tauri::RunEvent::Reopen { .. } => {
            if let Some(state) = app_handle.try_state::<LauncherState>() {
                state.request_open();
            }
        }
        tauri::RunEvent::Exit | tauri::RunEvent::ExitRequested { .. } => {
            if let Some(state) = app_handle.try_state::<LauncherState>() {
                state.terminate();
            }
        }
        _ => {}
    });
}

struct LauncherState {
    child: Arc<Mutex<Option<Child>>>,
    ready: Mutex<bool>,
    open_requested: Mutex<bool>,
    terminating: Mutex<bool>,
    base_url: String,
    access_url: String,
    log_path: PathBuf,
    #[cfg(feature = "desktop-updater")]
    update_check_in_progress: Arc<AtomicBool>,
}

impl LauncherState {
    fn new(base_url: String, access_url: String, log_path: PathBuf) -> Self {
        Self {
            child: Arc::new(Mutex::new(None)),
            ready: Mutex::new(false),
            open_requested: Mutex::new(false),
            terminating: Mutex::new(false),
            base_url,
            access_url,
            log_path,
            #[cfg(feature = "desktop-updater")]
            update_check_in_progress: Arc::new(AtomicBool::new(false)),
        }
    }

    #[cfg(feature = "desktop-updater")]
    fn begin_update_check(&self) -> Option<UpdateCheckGuard> {
        self.update_check_in_progress
            .compare_exchange(false, true, Ordering::AcqRel, Ordering::Acquire)
            .ok()
            .map(|_| UpdateCheckGuard(self.update_check_in_progress.clone()))
    }

    fn request_open(&self) {
        if *self.ready.lock().unwrap() {
            match open_system_url(&self.access_url) {
                Ok(()) => write_log(&self.log_path, &browser_opened_log_message(&self.base_url)),
                Err(error) => {
                    write_log(&self.log_path, &format!("failed to open browser: {error}"))
                }
            }
            return;
        }

        *self.open_requested.lock().unwrap() = true;
    }

    fn mark_ready(&self) {
        *self.ready.lock().unwrap() = true;
        let should_open = {
            let mut requested = self.open_requested.lock().unwrap();
            let value = *requested;
            *requested = false;
            value
        };

        if should_open {
            self.request_open();
        }
    }

    fn terminate(&self) {
        *self.terminating.lock().unwrap() = true;
        let mut guard = self.child.lock().unwrap();
        if let Some(child) = guard.as_mut() {
            match stop_child(child) {
                Ok(()) => write_log(&self.log_path, "terminated Lumi backend"),
                Err(error) => write_log(
                    &self.log_path,
                    &format!("failed to terminate Lumi backend: {error}"),
                ),
            }
        }
        *guard = None;
    }

    fn is_terminating(&self) -> bool {
        *self.terminating.lock().unwrap()
    }

    fn attach_child(&self, mut child: Child) -> bool {
        let mut guard = self.child.lock().unwrap();
        if self.is_terminating() {
            drop(guard);
            match stop_child(&mut child) {
                Ok(()) => write_log(
                    &self.log_path,
                    "terminated Lumi backend started during shutdown",
                ),
                Err(error) => write_log(
                    &self.log_path,
                    &format!("failed to terminate backend started during shutdown: {error}"),
                ),
            }
            return false;
        }
        *guard = Some(child);
        true
    }
}

fn build_tray(app_handle: &AppHandle) -> tauri::Result<()> {
    let open_item = MenuItem::with_id(app_handle, "open", "Open Lumi", true, Some("CmdOrCtrl+O"))?;
    let copy_url_item = MenuItem::with_id(
        app_handle,
        "copy-url",
        "Copy Access URL",
        true,
        Some("CmdOrCtrl+C"),
    )?;
    let logs_item = MenuItem::with_id(
        app_handle,
        "view-logs",
        "View Logs",
        true,
        Some("CmdOrCtrl+L"),
    )?;
    let quit_item = MenuItem::with_id(app_handle, "quit", "Quit", true, Some("CmdOrCtrl+Q"))?;

    #[cfg(feature = "desktop-updater")]
    let menu = {
        let check_updates_item = MenuItem::with_id(
            app_handle,
            "check-updates",
            "Check for Updates…",
            true,
            None::<&str>,
        )?;
        Menu::with_items(
            app_handle,
            &[
                &open_item,
                &copy_url_item,
                &logs_item,
                &PredefinedMenuItem::separator(app_handle)?,
                &check_updates_item,
                &PredefinedMenuItem::separator(app_handle)?,
                &quit_item,
            ],
        )?
    };

    #[cfg(not(feature = "desktop-updater"))]
    let menu = Menu::with_items(
        app_handle,
        &[
            &open_item,
            &copy_url_item,
            &logs_item,
            &PredefinedMenuItem::separator(app_handle)?,
            &quit_item,
        ],
    )?;

    let mut tray = TrayIconBuilder::with_id(TRAY_ID)
        .tooltip(APP_NAME)
        .menu(&menu)
        .show_menu_on_left_click(true)
        .on_menu_event(|app, event| match event.id.as_ref() {
            "open" => app.state::<LauncherState>().request_open(),
            "copy-url" => {
                let state = app.state::<LauncherState>();
                if let Err(error) = app.clipboard().write_text(state.access_url.clone()) {
                    write_log(&state.log_path, &format!("failed to copy URL: {error}"));
                }
            }
            "view-logs" => {
                let state = app.state::<LauncherState>();
                if let Err(error) = open_system_target(&state.log_path) {
                    write_log(&state.log_path, &format!("failed to open logs: {error}"));
                }
            }
            #[cfg(feature = "desktop-updater")]
            "check-updates" => {
                check_for_updates(app.clone(), UpdateCheckOrigin::Manual);
            }
            "quit" => {
                app.state::<LauncherState>().terminate();
                app.exit(0);
            }
            _ => {}
        });

    if let Some(icon) = app_handle.default_window_icon() {
        tray = tray.icon(icon.clone());
    }
    tray.build(app_handle)?;
    Ok(())
}

#[cfg(feature = "desktop-updater")]
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
enum UpdateCheckOrigin {
    Startup,
    Manual,
}

#[cfg(feature = "desktop-updater")]
impl UpdateCheckOrigin {
    fn reports_no_update(self) -> bool {
        self == Self::Manual
    }

    fn reports_errors(self) -> bool {
        self == Self::Manual
    }
}

#[cfg(feature = "desktop-updater")]
struct UpdateCheckGuard(Arc<AtomicBool>);

#[cfg(feature = "desktop-updater")]
impl Drop for UpdateCheckGuard {
    fn drop(&mut self) {
        self.0.store(false, Ordering::Release);
    }
}

#[cfg(feature = "desktop-updater")]
#[derive(Debug, PartialEq, Eq)]
enum UpdateFailure {
    Check(String),
    Install(String),
}

#[cfg(feature = "desktop-updater")]
impl UpdateFailure {
    fn check(error: impl ToString) -> Self {
        Self::Check(error.to_string())
    }

    fn install(error: impl ToString) -> Self {
        Self::Install(error.to_string())
    }

    fn reports_to_user(&self, origin: UpdateCheckOrigin) -> bool {
        matches!(self, Self::Install(_)) || origin.reports_errors()
    }

    fn title(&self) -> &'static str {
        match self {
            Self::Check(_) => "Update Check Failed",
            Self::Install(_) => "Update Failed",
        }
    }

    fn user_message(&self) -> String {
        match self {
            Self::Check(error) => format!("Lumi could not check for updates.\n\n{error}"),
            Self::Install(error) => format!(
                "Lumi could not install the update. The current version is still installed.\n\n{error}"
            ),
        }
    }
}

#[cfg(feature = "desktop-updater")]
impl std::fmt::Display for UpdateFailure {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::Check(error) => write!(formatter, "update check failed: {error}"),
            Self::Install(error) => write!(formatter, "update installation failed: {error}"),
        }
    }
}

#[cfg(feature = "desktop-updater")]
fn check_for_updates(app: AppHandle, origin: UpdateCheckOrigin) {
    let Some(guard) = app.state::<LauncherState>().begin_update_check() else {
        if origin == UpdateCheckOrigin::Manual {
            app.dialog()
                .message("Lumi is already checking for updates.")
                .kind(MessageDialogKind::Info)
                .title("Update Check in Progress")
                .blocking_show();
        }
        return;
    };

    tauri::async_runtime::spawn(async move {
        let _guard = guard;
        if let Err(error) = run_update_check(&app, origin).await {
            let log_path = app.state::<LauncherState>().log_path.clone();
            write_log(&log_path, &error.to_string());
            if error.reports_to_user(origin) {
                show_error(&app, error.title(), error.user_message());
            }
        }
    });
}

#[cfg(feature = "desktop-updater")]
async fn run_update_check(app: &AppHandle, origin: UpdateCheckOrigin) -> Result<(), UpdateFailure> {
    let exit_handle = app.clone();
    let updater = app
        .updater_builder()
        .on_before_exit(move || {
            if let Some(state) = exit_handle.try_state::<LauncherState>() {
                state.terminate();
            }
            exit_handle.cleanup_before_exit();
        })
        .build()
        .map_err(UpdateFailure::check)?;

    let Some(update) = updater.check().await.map_err(UpdateFailure::check)? else {
        if origin.reports_no_update() {
            app.dialog()
                .message(format!(
                    "You're running the latest version:\n\nv{}",
                    app.package_info().version
                ))
                .kind(MessageDialogKind::Info)
                .title("No Updates Available")
                .blocking_show();
        }
        return Ok(());
    };

    let should_install = app
        .dialog()
        .message(format!(
            "Version {} is available!\n\nWould you like to download and install it now?",
            update.version
        ))
        .kind(MessageDialogKind::Info)
        .title("Update Available")
        .buttons(MessageDialogButtons::OkCancel)
        .blocking_show();

    if !should_install {
        return Ok(());
    }

    let log_path = app.state::<LauncherState>().log_path.clone();
    write_log(
        &log_path,
        &format!("downloading Lumi update {}", update.version),
    );
    let finished_log_path = log_path.clone();
    update
        .download_and_install(
            |_, _| {},
            move || write_log(&finished_log_path, "finished downloading Lumi update"),
        )
        .await
        .map_err(UpdateFailure::install)?;

    write_log(&log_path, "installed Lumi update; restarting");
    app.state::<LauncherState>().terminate();
    app.restart();
}

fn launch_and_monitor_backend(app: AppHandle, port: u16, access_token: String, log_path: PathBuf) {
    if app.state::<LauncherState>().is_terminating() {
        return;
    }

    let child = match start_backend(&app, port, &access_token, &log_path) {
        Ok(child) => child,
        Err(error) => {
            startup_failed(&app, &log_path, error);
            return;
        }
    };

    let child_slot = app.state::<LauncherState>().child.clone();
    if !app.state::<LauncherState>().attach_child(child) {
        return;
    }

    if let Err(error) = wait_until_ready(&child_slot, port, STARTUP_TIMEOUT) {
        app.state::<LauncherState>().terminate();
        startup_failed(&app, &log_path, error);
        return;
    }

    write_log(
        &log_path,
        &format!("Lumi backend ready at {}", app_url(port)),
    );
    app.state::<LauncherState>().mark_ready();
    monitor_backend(app, child_slot, log_path);
}

fn start_backend(
    app: &AppHandle,
    port: u16,
    access_token: &str,
    log_path: &Path,
) -> Result<Child, String> {
    let backend_path = if cfg!(debug_assertions) {
        std::env::var_os("LUMI_DESKTOP_BACKEND")
            .map(PathBuf::from)
            .unwrap_or_else(default_debug_backend_path)
    } else {
        bundled_backend_path(
            &app.path()
                .resource_dir()
                .map_err(|error| error.to_string())?,
        )
    };

    if !backend_path.is_file() {
        return Err(format!(
            "bundled Lumi backend was not found at {}",
            backend_path.display()
        ));
    }

    let home = app
        .path()
        .home_dir()
        .map_err(|error| format!("failed to resolve the user home directory: {error}"))?;
    let address = format!("127.0.0.1:{port}");
    let frontend_url = app_url(port);
    write_log(log_path, &format!("starting Lumi backend on {address}"));

    let mut command = Command::new(&backend_path);
    configure_backend_command(&mut command, &home, &address, &frontend_url, access_token);
    let mut child = command
        .spawn()
        .map_err(|error| format!("failed to start {}: {error}", backend_path.display()))?;

    if let Some(stdout) = child.stdout.take() {
        spawn_output_thread(stdout, log_path.to_path_buf(), "stdout");
    }
    if let Some(stderr) = child.stderr.take() {
        spawn_output_thread(stderr, log_path.to_path_buf(), "stderr");
    }
    Ok(child)
}

fn default_debug_backend_path() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join(DEBUG_BACKEND_DIRECTORY)
        .join(BACKEND_BINARY_NAME)
}

fn bundled_backend_path(resource_dir: &Path) -> PathBuf {
    resource_dir.join("backend").join(BACKEND_BINARY_NAME)
}

fn configure_backend_command(
    command: &mut Command,
    home: &Path,
    address: &str,
    frontend_url: &str,
    access_token: &str,
) {
    command
        .current_dir(home)
        .env("APP_ENV", "production")
        .env("APP_ADDRESS", address)
        .env("FRONTEND_URL", frontend_url)
        .env(DESKTOP_ACCESS_TOKEN_ENV, access_token)
        .stdin(Stdio::null())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped());
    configure_hidden_command(command);
}

#[cfg(target_os = "windows")]
fn configure_hidden_command(command: &mut Command) {
    command.creation_flags(CREATE_NO_WINDOW);
}

#[cfg(not(target_os = "windows"))]
fn configure_hidden_command(_command: &mut Command) {}

fn wait_until_ready(
    child_slot: &Arc<Mutex<Option<Child>>>,
    port: u16,
    timeout: Duration,
) -> Result<(), String> {
    let deadline = Instant::now() + timeout;
    loop {
        if health_is_ready(port).unwrap_or(false) {
            return Ok(());
        }

        {
            let mut guard = child_slot.lock().unwrap();
            let child = guard
                .as_mut()
                .ok_or_else(|| "Lumi backend process disappeared during startup".to_string())?;
            if let Some(status) = child
                .try_wait()
                .map_err(|error| format!("failed to inspect Lumi backend: {error}"))?
            {
                *guard = None;
                return Err(format!("Lumi backend exited during startup: {status}"));
            }
        }

        if Instant::now() >= deadline {
            return Err(format!(
                "Lumi backend did not become healthy within {} seconds",
                timeout.as_secs()
            ));
        }
        std::thread::sleep(HEALTH_INTERVAL);
    }
}

fn monitor_backend(app: AppHandle, child_slot: Arc<Mutex<Option<Child>>>, log_path: PathBuf) {
    loop {
        if app.state::<LauncherState>().is_terminating() {
            return;
        }

        let status = {
            let mut guard = child_slot.lock().unwrap();
            match guard.as_mut() {
                Some(child) => match child.try_wait() {
                    Ok(Some(status)) => {
                        *guard = None;
                        Some(Ok(status))
                    }
                    Ok(None) => None,
                    Err(error) => Some(Err(error)),
                },
                None => return,
            }
        };

        match status {
            Some(Ok(status)) => {
                let code = status.code().unwrap_or(1);
                write_log(
                    &log_path,
                    &format!("Lumi backend exited with status {code}"),
                );
                if code != 0 {
                    show_error(
                        &app,
                        "Lumi Exited",
                        format!(
                            "The Lumi backend exited with code {code}.\n\nLogs: {}",
                            log_path.display()
                        ),
                    );
                }
                app.exit(code);
                return;
            }
            Some(Err(error)) => {
                write_log(&log_path, &format!("failed to monitor backend: {error}"));
                app.exit(1);
                return;
            }
            None => std::thread::sleep(Duration::from_millis(250)),
        }
    }
}

fn startup_failed(app: &AppHandle, log_path: &Path, error: String) {
    write_log(log_path, &format!("Lumi startup failed: {error}"));
    show_error(
        app,
        "Lumi Failed to Start",
        format!("{error}\n\nLogs: {}", log_path.display()),
    );
    app.exit(1);
}

fn show_error(app: &AppHandle, title: &str, message: String) {
    app.dialog()
        .message(message)
        .kind(MessageDialogKind::Error)
        .title(title)
        .blocking_show();
}

fn app_url(port: u16) -> String {
    format!("http://127.0.0.1:{port}")
}

fn desktop_access_url(base_url: &str, access_token: &str) -> String {
    format!("{base_url}/#desktop_token={access_token}")
}

fn browser_opened_log_message(base_url: &str) -> String {
    format!("opened Lumi in system browser at {base_url}")
}

fn generate_access_token() -> Result<String, String> {
    let mut bytes = [0_u8; 32];
    getrandom::fill(&mut bytes)
        .map_err(|error| format!("failed to generate desktop access token: {error}"))?;
    Ok(URL_SAFE_NO_PAD.encode(bytes))
}

fn select_port() -> io::Result<u16> {
    let listener = TcpListener::bind(SocketAddrV4::new(Ipv4Addr::LOCALHOST, 0))?;
    let port = listener.local_addr()?.port();
    drop(listener);
    Ok(port)
}

fn health_is_ready(port: u16) -> io::Result<bool> {
    let address = SocketAddr::V4(SocketAddrV4::new(Ipv4Addr::LOCALHOST, port));
    let mut stream = TcpStream::connect_timeout(&address, Duration::from_millis(300))?;
    stream.set_read_timeout(Some(Duration::from_millis(500)))?;
    stream.set_write_timeout(Some(Duration::from_millis(500)))?;
    stream.write_all(
        b"GET /api/v1/health HTTP/1.1\r\nHost: 127.0.0.1\r\nConnection: close\r\n\r\n",
    )?;

    let mut response = String::new();
    stream.read_to_string(&mut response)?;
    Ok(parse_health_response(&response))
}

fn parse_health_response(response: &str) -> bool {
    let Some((headers, body)) = response.split_once("\r\n\r\n") else {
        return false;
    };
    let Some(status_line) = headers.lines().next() else {
        return false;
    };
    if !status_line
        .split_whitespace()
        .nth(1)
        .is_some_and(|code| code == "200")
    {
        return false;
    }

    let Ok(value) = serde_json::from_str::<serde_json::Value>(body) else {
        return false;
    };
    value.get("success").and_then(|item| item.as_bool()) == Some(true)
        && value.pointer("/data/status").and_then(|item| item.as_str()) == Some("ok")
        && value
            .pointer("/data/database")
            .and_then(|item| item.as_str())
            == Some("connected")
}

fn stop_child(child: &mut Child) -> io::Result<()> {
    if child.try_wait()?.is_some() {
        return Ok(());
    }

    #[cfg(target_os = "windows")]
    {
        child.kill()?;
        child.wait()?;
        return Ok(());
    }

    #[cfg(unix)]
    {
        unsafe {
            libc::kill(child.id() as libc::pid_t, libc::SIGTERM);
        }

        let deadline = Instant::now() + Duration::from_secs(5);
        while Instant::now() < deadline {
            if child.try_wait()?.is_some() {
                return Ok(());
            }
            std::thread::sleep(Duration::from_millis(50));
        }

        child.kill()?;
        child.wait()?;
        return Ok(());
    }

    #[cfg(not(any(unix, target_os = "windows")))]
    {
        child.kill()?;
        child.wait()?;
        Ok(())
    }
}

#[cfg(target_os = "macos")]
fn open_system_target(target: impl AsRef<std::ffi::OsStr>) -> io::Result<()> {
    let status = Command::new("/usr/bin/open").arg(target).status()?;
    if status.success() {
        Ok(())
    } else {
        Err(io::Error::other(format!(
            "/usr/bin/open exited with status {status}"
        )))
    }
}

#[cfg(target_os = "windows")]
fn open_system_target(target: impl AsRef<OsStr>) -> io::Result<()> {
    let mut command = Command::new("powershell.exe");
    command
        .args([
            "-NoLogo",
            "-NoProfile",
            "-NonInteractive",
            "-WindowStyle",
            "Hidden",
            "-Command",
            OPEN_TARGET_SCRIPT,
        ])
        .env(OPEN_TARGET_ENV, target.as_ref())
        .stdin(Stdio::null())
        .stdout(Stdio::null())
        .stderr(Stdio::null());
    configure_hidden_command(&mut command);

    let status = command.status()?;
    if status.success() {
        Ok(())
    } else {
        Err(io::Error::other(format!(
            "PowerShell Start-Process exited with status {status}"
        )))
    }
}

#[cfg(not(any(target_os = "macos", target_os = "windows")))]
fn open_system_target(target: impl AsRef<std::ffi::OsStr>) -> io::Result<()> {
    let status = Command::new("xdg-open").arg(target).status()?;
    if status.success() {
        Ok(())
    } else {
        Err(io::Error::other(format!(
            "xdg-open exited with status {status}"
        )))
    }
}

#[cfg(target_os = "macos")]
fn open_system_url(target: &str) -> io::Result<()> {
    use objc2_app_kit::NSWorkspace;
    use objc2_foundation::{NSString, NSURL};

    let url = NSURL::URLWithString(&NSString::from_str(target))
        .ok_or_else(|| io::Error::new(io::ErrorKind::InvalidInput, "invalid browser URL"))?;
    if NSWorkspace::sharedWorkspace().openURL(&url) {
        Ok(())
    } else {
        Err(io::Error::other("NSWorkspace failed to open browser URL"))
    }
}

#[cfg(not(target_os = "macos"))]
fn open_system_url(target: &str) -> io::Result<()> {
    open_system_target(target)
}

fn spawn_output_thread<R>(mut stream: R, log_path: PathBuf, label: &'static str)
where
    R: Read + Send + 'static,
{
    std::thread::spawn(move || {
        let mut buffer = [0_u8; 4096];
        loop {
            match stream.read(&mut buffer) {
                Ok(0) => break,
                Ok(count) => {
                    let text = String::from_utf8_lossy(&buffer[..count]);
                    write_log(&log_path, &format!("[{label}] {}", text.trim_end()));
                }
                Err(error) => {
                    write_log(&log_path, &format!("failed to read {label}: {error}"));
                    break;
                }
            }
        }
    });
}

fn log_file_in(directory: PathBuf) -> PathBuf {
    directory.join("lumi.log")
}

#[cfg(target_os = "windows")]
fn log_path(app: &AppHandle) -> tauri::Result<PathBuf> {
    Ok(log_file_in(app.path().app_log_dir()?))
}

#[cfg(not(target_os = "windows"))]
fn log_path(app: &AppHandle) -> tauri::Result<PathBuf> {
    let home = app.path().home_dir()?;
    Ok(log_file_in(
        home.join("Library").join("Logs").join(APP_NAME),
    ))
}

fn ensure_parent_dir(path: &Path) -> io::Result<()> {
    if let Some(parent) = path.parent() {
        fs::create_dir_all(parent)?;
    }
    Ok(())
}

fn write_log(path: &Path, message: &str) {
    let timestamp = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|value| value.as_secs())
        .unwrap_or(0);
    if let Some(parent) = path.parent() {
        let _ = fs::create_dir_all(parent);
    }
    if let Ok(mut file) = OpenOptions::new().create(true).append(true).open(path) {
        let _ = writeln!(file, "[{timestamp}] {message}");
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::thread;

    #[cfg(unix)]
    fn long_running_test_command() -> Command {
        let mut command = Command::new("/bin/sleep");
        command.arg("30");
        command
    }

    #[cfg(target_os = "windows")]
    fn long_running_test_command() -> Command {
        let mut command = Command::new("powershell.exe");
        command.args([
            "-NoLogo",
            "-NoProfile",
            "-NonInteractive",
            "-Command",
            "Start-Sleep -Seconds 30",
        ]);
        configure_hidden_command(&mut command);
        command
    }

    #[cfg(unix)]
    fn exiting_test_command() -> Command {
        let mut command = Command::new("/bin/sh");
        command.args(["-c", "exit 7"]);
        command
    }

    #[cfg(target_os = "windows")]
    fn exiting_test_command() -> Command {
        let mut command = Command::new("cmd.exe");
        command.args(["/C", "exit 7"]);
        configure_hidden_command(&mut command);
        command
    }

    #[cfg(unix)]
    fn assert_process_is_stopped(pid: u32) {
        assert_eq!(unsafe { libc::kill(pid as libc::pid_t, 0) }, -1);
    }

    #[cfg(target_os = "windows")]
    fn assert_process_is_stopped(pid: u32) {
        let mut command = Command::new("powershell.exe");
        command
            .args([
                "-NoLogo",
                "-NoProfile",
                "-NonInteractive",
                "-Command",
                "if (Get-Process -Id $env:LUMI_TEST_PID -ErrorAction SilentlyContinue) { exit 1 }",
            ])
            .env("LUMI_TEST_PID", pid.to_string());
        configure_hidden_command(&mut command);
        assert!(command.status().unwrap().success());
    }

    #[test]
    fn app_url_uses_loopback() {
        assert_eq!(app_url(49152), "http://127.0.0.1:49152");
    }

    #[test]
    fn desktop_access_tokens_are_random_url_safe_values() {
        let first = generate_access_token().unwrap();
        let second = generate_access_token().unwrap();
        assert_ne!(first, second);
        assert_eq!(URL_SAFE_NO_PAD.decode(first).unwrap().len(), 32);
        assert_eq!(URL_SAFE_NO_PAD.decode(second).unwrap().len(), 32);
    }

    #[test]
    fn desktop_access_url_keeps_the_token_in_the_fragment() {
        let base_url = app_url(49152);
        let url = desktop_access_url(&base_url, "secret-token");
        assert_eq!(url, "http://127.0.0.1:49152/#desktop_token=secret-token");
        assert!(!base_url.contains("secret-token"));
    }

    #[test]
    fn browser_open_log_message_does_not_include_the_access_token() {
        let base_url = app_url(49152);
        let access_url = desktop_access_url(&base_url, "secret-token");
        let message = browser_opened_log_message(&base_url);

        assert_eq!(
            message,
            "opened Lumi in system browser at http://127.0.0.1:49152"
        );
        assert!(!message.contains("secret-token"));
        assert!(!message.contains(&access_url));
    }

    #[test]
    fn backend_command_receives_the_desktop_token_only_in_its_environment() {
        let mut command = Command::new("lumi-test-backend");
        configure_backend_command(
            &mut command,
            &std::env::temp_dir(),
            "127.0.0.1:49152",
            "http://127.0.0.1:49152",
            "secret-token",
        );
        let environment = command
            .get_envs()
            .filter_map(|(key, value)| value.map(|value| (key.to_owned(), value.to_owned())))
            .collect::<std::collections::HashMap<_, _>>();
        assert_eq!(
            environment.get(std::ffi::OsStr::new(DESKTOP_ACCESS_TOKEN_ENV)),
            Some(&std::ffi::OsString::from("secret-token"))
        );
        assert!(!command
            .get_args()
            .any(|argument| argument.to_string_lossy().contains("secret-token")));
    }

    #[test]
    fn selected_port_is_loopback_and_available_after_selection() {
        let (port, listener) = (0..10)
            .find_map(|_| {
                let port = select_port().unwrap();
                TcpListener::bind((Ipv4Addr::LOCALHOST, port))
                    .ok()
                    .map(|listener| (port, listener))
            })
            .expect("selected loopback ports remained unavailable after 10 attempts");
        assert_eq!(listener.local_addr().unwrap().ip(), Ipv4Addr::LOCALHOST);
        assert_eq!(listener.local_addr().unwrap().port(), port);
    }

    #[test]
    fn health_parser_requires_the_complete_success_contract() {
        let good = "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n{\"success\":true,\"data\":{\"status\":\"ok\",\"database\":\"connected\"}}";
        assert!(parse_health_response(good));
        assert!(!parse_health_response(&good.replace("connected", "failed")));
        assert!(!parse_health_response(
            &good.replace("200 OK", "503 Unavailable")
        ));
    }

    #[test]
    fn tauri_config_has_no_webview_window_or_file_association() {
        let config: serde_json::Value =
            serde_json::from_str(include_str!("../tauri.conf.json")).unwrap();
        assert_eq!(
            config
                .pointer("/productName")
                .and_then(|item| item.as_str()),
            Some("Lumi")
        );
        assert_eq!(
            config.pointer("/identifier").and_then(|item| item.as_str()),
            Some("dev.lumi.Lumi")
        );
        assert!(config
            .pointer("/app/windows")
            .and_then(|item| item.as_array())
            .is_some_and(|windows| windows.is_empty()));
        assert!(config.pointer("/bundle/fileAssociations").is_none());
        assert_eq!(
            config
                .pointer("/plugins/updater/endpoints/0")
                .and_then(|item| item.as_str()),
            Some("https://github.com/lumikaka/lumi/releases/latest/download/latest.json")
        );
        assert!(config
            .pointer("/plugins/updater/pubkey")
            .and_then(|item| item.as_str())
            .is_some_and(|pubkey| !pubkey.is_empty()));
        assert_eq!(
            config
                .pointer("/plugins/updater/windows/installMode")
                .and_then(|item| item.as_str()),
            Some("passive")
        );
    }

    #[cfg(feature = "desktop-updater")]
    #[test]
    fn update_check_origin_only_reports_manual_no_update_and_errors() {
        assert!(!UpdateCheckOrigin::Startup.reports_no_update());
        assert!(!UpdateCheckOrigin::Startup.reports_errors());
        assert!(UpdateCheckOrigin::Manual.reports_no_update());
        assert!(UpdateCheckOrigin::Manual.reports_errors());
    }

    #[cfg(feature = "desktop-updater")]
    #[test]
    fn update_failures_only_hide_startup_check_errors() {
        let check_failure = UpdateFailure::check("offline");
        assert!(!check_failure.reports_to_user(UpdateCheckOrigin::Startup));
        assert!(check_failure.reports_to_user(UpdateCheckOrigin::Manual));
        assert_eq!(check_failure.title(), "Update Check Failed");

        let install_failure = UpdateFailure::install("signature rejected");
        assert!(install_failure.reports_to_user(UpdateCheckOrigin::Startup));
        assert!(install_failure.reports_to_user(UpdateCheckOrigin::Manual));
        assert_eq!(install_failure.title(), "Update Failed");
        assert!(install_failure
            .user_message()
            .contains("current version is still installed"));
    }

    #[cfg(feature = "desktop-updater")]
    #[test]
    fn update_check_guard_prevents_concurrent_checks_and_resets_on_drop() {
        let log_path = std::env::temp_dir().join("lumi-update-check-guard.log");
        let base_url = app_url(49152);
        let state = LauncherState::new(
            base_url.clone(),
            desktop_access_url(&base_url, "secret-token"),
            log_path,
        );

        let guard = state.begin_update_check().unwrap();
        assert!(state.begin_update_check().is_none());
        drop(guard);
        assert!(state.begin_update_check().is_some());
    }

    #[test]
    fn windows_tauri_config_builds_only_an_unsigned_current_user_nsis_installer() {
        let config: serde_json::Value =
            serde_json::from_str(include_str!("../tauri.windows.conf.json")).unwrap();
        let targets = config.pointer("/bundle/targets").unwrap();
        assert_eq!(targets, &serde_json::json!(["nsis"]));
        let icons = config.pointer("/bundle/icon").unwrap();
        assert_eq!(
            icons,
            &serde_json::json!(["icons/windows-generated/icon.ico"])
        );
        assert_eq!(
            config
                .pointer("/bundle/windows/nsis/installMode")
                .and_then(|item| item.as_str()),
            Some("currentUser")
        );
        assert_eq!(
            config
                .pointer("/bundle/windows/webviewInstallMode/type")
                .and_then(|item| item.as_str()),
            Some("skip")
        );
        assert_eq!(
            config
                .pointer("/bundle/windows/nsis/installerIcon")
                .and_then(|item| item.as_str()),
            Some("icons/windows-generated/icon.ico")
        );
        assert_eq!(
            config
                .pointer("/bundle/createUpdaterArtifacts")
                .and_then(|item| item.as_bool()),
            Some(false)
        );
        for pointer in [
            "/bundle/windows/certificateThumbprint",
            "/bundle/windows/digestAlgorithm",
            "/bundle/windows/signCommand",
            "/bundle/windows/timestampUrl",
            "/bundle/windows/tsp",
        ] {
            assert!(config.pointer(pointer).is_none());
        }
        assert!(config.pointer("/bundle/fileAssociations").is_none());
    }

    #[cfg(target_os = "windows")]
    #[test]
    fn windows_paths_use_exe_and_application_log_directory() {
        let resources = PathBuf::from(r"C:\Program Files\Lumi");
        let local_app_data = PathBuf::from(r"C:\Users\Lumi\AppData\Local");
        let app_log_dir = local_app_data.join("dev.lumi.Lumi").join("logs");
        let expected_log_path = app_log_dir.join("lumi.log");
        assert_eq!(
            bundled_backend_path(&resources),
            resources.join("backend").join("lumi_web.exe")
        );
        assert_eq!(BACKEND_BINARY_NAME, "lumi_web.exe");
        assert_eq!(DEBUG_BACKEND_DIRECTORY, "backend-windows");
        assert_eq!(log_file_in(app_log_dir), expected_log_path);
    }

    #[cfg(target_os = "windows")]
    #[test]
    fn windows_open_target_passes_the_value_only_through_the_environment() {
        let target = r"https://127.0.0.1/#desktop_token=secret-token";
        let mut command = Command::new("powershell.exe");
        command
            .args([
                "-NoLogo",
                "-NoProfile",
                "-NonInteractive",
                "-WindowStyle",
                "Hidden",
                "-Command",
                OPEN_TARGET_SCRIPT,
            ])
            .env(OPEN_TARGET_ENV, target);

        assert_eq!(
            command
                .get_envs()
                .find(|(key, _)| *key == std::ffi::OsStr::new(OPEN_TARGET_ENV))
                .and_then(|(_, value)| value),
            Some(std::ffi::OsStr::new(target))
        );
        assert!(!command
            .get_args()
            .any(|argument| argument.to_string_lossy().contains("secret-token")));
    }

    #[test]
    fn health_check_reads_a_loopback_server() {
        let listener = TcpListener::bind((Ipv4Addr::LOCALHOST, 0)).unwrap();
        let port = listener.local_addr().unwrap().port();
        let server = thread::spawn(move || {
            let (mut stream, _) = listener.accept().unwrap();
            let mut request = [0_u8; 512];
            let count = stream.read(&mut request).unwrap();
            assert!(String::from_utf8_lossy(&request[..count]).starts_with("GET /api/v1/health"));
            let body = "{\"success\":true,\"data\":{\"status\":\"ok\",\"database\":\"connected\"}}";
            write!(
                stream,
                "HTTP/1.1 200 OK\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
                body.len(),
                body
            )
            .unwrap();
        });

        assert!(health_is_ready(port).unwrap());
        server.join().unwrap();
    }

    #[test]
    fn startup_wait_retries_until_health_is_ready() {
        let listener = TcpListener::bind((Ipv4Addr::LOCALHOST, 0)).unwrap();
        listener.set_nonblocking(true).unwrap();
        let port = listener.local_addr().unwrap().port();
        let server = thread::spawn(move || {
            let deadline = Instant::now() + Duration::from_secs(2);
            let mut attempts = 0;
            while attempts < 2 && Instant::now() < deadline {
                match listener.accept() {
                    Ok((mut stream, _)) => {
                        attempts += 1;
                        let mut request = [0_u8; 512];
                        let count = stream.read(&mut request).unwrap();
                        assert!(String::from_utf8_lossy(&request[..count])
                            .starts_with("GET /api/v1/health"));

                        let status = if attempts == 1 {
                            "503 Service Unavailable"
                        } else {
                            "200 OK"
                        };
                        let body =
                            "{\"success\":true,\"data\":{\"status\":\"ok\",\"database\":\"connected\"}}";
                        write!(
                            stream,
                            "HTTP/1.1 {status}\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body}",
                            body.len()
                        )
                        .unwrap();
                    }
                    Err(error) if error.kind() == io::ErrorKind::WouldBlock => {
                        thread::sleep(Duration::from_millis(10));
                    }
                    Err(error) => panic!("health test server failed: {error}"),
                }
            }
            attempts
        });

        let child = long_running_test_command().spawn().unwrap();
        let child_slot = Arc::new(Mutex::new(Some(child)));
        let result = wait_until_ready(&child_slot, port, Duration::from_secs(2));
        let mut child = child_slot.lock().unwrap().take().unwrap();
        stop_child(&mut child).unwrap();

        assert!(result.is_ok(), "startup wait failed: {result:?}");
        assert_eq!(server.join().unwrap(), 2);
    }

    #[test]
    fn startup_wait_reports_backend_exit() {
        let listener = TcpListener::bind((Ipv4Addr::LOCALHOST, 0)).unwrap();
        let port = listener.local_addr().unwrap().port();
        let child = exiting_test_command().spawn().unwrap();
        let child_slot = Arc::new(Mutex::new(Some(child)));

        let error = wait_until_ready(&child_slot, port, Duration::from_secs(2)).unwrap_err();

        assert!(error.contains("Lumi backend exited during startup"));
        assert!(child_slot.lock().unwrap().is_none());
    }

    #[test]
    fn stop_child_terminates_a_running_process() {
        let mut child = long_running_test_command().spawn().unwrap();
        let pid = child.id();
        stop_child(&mut child).unwrap();
        assert_process_is_stopped(pid);
    }

    #[test]
    fn child_started_during_shutdown_is_not_attached() {
        let log_path = std::env::temp_dir().join(format!(
            "lumi-desktop-shutdown-{}-{}.log",
            std::process::id(),
            SystemTime::now()
                .duration_since(UNIX_EPOCH)
                .unwrap()
                .as_nanos()
        ));
        let base_url = app_url(49152);
        let access_url = desktop_access_url(&base_url, "secret-token");
        let state = LauncherState::new(base_url, access_url, log_path);
        state.terminate();

        let child = long_running_test_command().spawn().unwrap();
        let pid = child.id();
        assert!(!state.attach_child(child));
        assert!(state.child.lock().unwrap().is_none());
        assert_process_is_stopped(pid);
    }
}
