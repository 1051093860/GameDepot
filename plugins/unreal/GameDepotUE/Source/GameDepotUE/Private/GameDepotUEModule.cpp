#include "GameDepotUEModule.h"

#include "ContentBrowserModule.h"
#include "AssetRegistry/AssetRegistryModule.h"
#include "ContentBrowserDelegates.h"
#include "Containers/Ticker.h"
#include "Dom/JsonObject.h"
#include "Framework/Application/SlateApplication.h"
#include "Framework/Docking/TabManager.h"
#include "Framework/MultiBox/MultiBoxBuilder.h"
#include "Framework/Notifications/NotificationManager.h"
#include "GameDepotConfigManager.h"
#include "GameDepotMockStatusProvider.h"
#include "SGameDepotOperationDialog.h"
#include "SGameDepotConflictDialog.h"
#include "HttpModule.h"
#include "IContentBrowserSingleton.h"
#include "Interfaces/IHttpRequest.h"
#include "Interfaces/IHttpResponse.h"
#include "HAL/FileManager.h"
#include "HAL/PlatformMisc.h"
#include "Misc/App.h"
#include "Misc/ConfigCacheIni.h"
#include "Misc/FileHelper.h"
#include "Misc/MessageDialog.h"
#include "FileHelpers.h"
#include "Misc/Paths.h"
#include "SGameDepotConfigPanel.h"
#include "SGameDepotHistoryDialog.h"
#include "SGameDepotStatusPanel.h"
#include "Serialization/JsonReader.h"
#include "Serialization/JsonSerializer.h"
#include "Styling/AppStyle.h"
#include "ToolMenus.h"
#include "Widgets/Docking/SDockTab.h"
#include "Widgets/Notifications/SNotificationList.h"
#include "Widgets/Input/SEditableTextBox.h"
#include "Widgets/Input/SButton.h"
#include "Widgets/Layout/SBorder.h"
#include "Widgets/SBoxPanel.h"
#include "Widgets/SWindow.h"
#include "Widgets/Text/STextBlock.h"

DEFINE_LOG_CATEGORY(LogGameDepotUE);
#define LOCTEXT_NAMESPACE "FGameDepotUEModule"

namespace
{
static const FName GameDepotStatusTabName(TEXT("GameDepotStatus"));
static const FName GameDepotConfigTabName(TEXT("GameDepotConfig"));

FString JsonEscape(const FString& In)
{
    FString Out = In;
    Out.ReplaceInline(TEXT("\\"), TEXT("\\\\"));
    Out.ReplaceInline(TEXT("\""), TEXT("\\\""));
    Out.ReplaceInline(TEXT("\r"), TEXT("\\r"));
    Out.ReplaceInline(TEXT("\n"), TEXT("\\n"));
    return Out;
}

TSharedRef<SWidget> MakeGameDepotMenuLabel(const FText& Text)
{
    return SNew(STextBlock).Text(Text).ToolTipText(Text);
}

bool ParseJsonObject(const FString& Text, TSharedPtr<FJsonObject>& Out)
{
    const TSharedRef<TJsonReader<>> Reader = TJsonReaderFactory<>::Create(Text);
    return FJsonSerializer::Deserialize(Reader, Out) && Out.IsValid();
}

bool TryGetStringField(const FString& JsonText, const FString& Field, FString& Out)
{
    TSharedPtr<FJsonObject> Obj;
    if (ParseJsonObject(JsonText, Obj))
    {
        return Obj->TryGetStringField(Field, Out);
    }
    return false;
}

bool TryGetObjectField(const FString& JsonText, const FString& Field, TSharedPtr<FJsonObject>& Out)
{
    TSharedPtr<FJsonObject> Obj;
    if (ParseJsonObject(JsonText, Obj))
    {
        const TSharedPtr<FJsonObject>* Found = nullptr;
        if (Obj->TryGetObjectField(Field, Found) && Found && Found->IsValid())
        {
            Out = *Found;
            return true;
        }
    }
    return false;
}

FString QuoteJSON(const FString& S)
{
    return FString::Printf(TEXT("\"%s\""), *JsonEscape(S));
}


FString TrimExecutableToken(FString Value)
{
    Value = Value.TrimStartAndEnd();
    if ((Value.StartsWith(TEXT("\"")) && Value.EndsWith(TEXT("\""))) ||
        (Value.StartsWith(TEXT("'")) && Value.EndsWith(TEXT("'"))))
    {
        Value = Value.Mid(1, Value.Len() - 2).TrimStartAndEnd();
    }
    return Value;
}

FString ResolveExecutableFromPATH(const FString& ExecutableName)
{
    FString Name = TrimExecutableToken(ExecutableName);
    if (Name.IsEmpty())
    {
        return FString();
    }

    if (FPaths::FileExists(Name))
    {
        return FPaths::ConvertRelativePathToFull(Name);
    }

    TArray<FString> NamesToTry;
    NamesToTry.AddUnique(Name);
#if PLATFORM_WINDOWS
    if (FPaths::GetExtension(Name, false).IsEmpty())
    {
        NamesToTry.AddUnique(Name + TEXT(".exe"));
    }
#endif

    FString PathValue = FPlatformMisc::GetEnvironmentVariable(TEXT("PATH"));
#if PLATFORM_WINDOWS
    if (PathValue.IsEmpty())
    {
        PathValue = FPlatformMisc::GetEnvironmentVariable(TEXT("Path"));
    }
    if (PathValue.IsEmpty())
    {
        PathValue = FPlatformMisc::GetEnvironmentVariable(TEXT("path"));
    }
#endif

    TArray<FString> PathDirs;
#if PLATFORM_WINDOWS
    PathValue.ParseIntoArray(PathDirs, TEXT(";"), true);
#else
    PathValue.ParseIntoArray(PathDirs, TEXT(":"), true);
#endif

    for (FString Dir : PathDirs)
    {
        Dir = TrimExecutableToken(Dir);
        if (Dir.IsEmpty())
        {
            continue;
        }

        for (const FString& CandidateName : NamesToTry)
        {
            const FString Candidate = FPaths::ConvertRelativePathToFull(FPaths::Combine(Dir, CandidateName));
            if (FPaths::FileExists(Candidate))
            {
                return Candidate;
            }
        }
    }

    return FString();
}

FString ResolveGameDepotExecutable(const FString& PreferredExecutable)
{
    if (!PreferredExecutable.TrimStartAndEnd().IsEmpty())
    {
        const FString ResolvedPreferred = ResolveExecutableFromPATH(PreferredExecutable);
        if (!ResolvedPreferred.IsEmpty())
        {
            return ResolvedPreferred;
        }
    }

    const FString ResolvedNormalName = ResolveExecutableFromPATH(TEXT("gamedepot.exe"));
    if (!ResolvedNormalName.IsEmpty())
    {
        return ResolvedNormalName;
    }

    // Tolerate the typo while local binaries or user settings are being renamed.
    const FString ResolvedTypoName = ResolveExecutableFromPATH(TEXT("gamedeport.exe"));
    if (!ResolvedTypoName.IsEmpty())
    {
        return ResolvedTypoName;
    }

    return FString();
}
}

void FGameDepotUEModule::StartupModule()
{
    ProjectRoot = FPaths::ConvertRelativePathToFull(FPaths::ProjectDir());
    LoadSettings();

    ConfigManager = MakeShared<FGameDepotConfigManager>();
    ConfigManager->Load();
    Provider = MakeShared<FGameDepotMockStatusProvider>();

    RegisterStatusTab();
    RegisterConfigTab();
    RegisterContentBrowserMenu();
    UToolMenus::RegisterStartupCallback(FSimpleMulticastDelegate::FDelegate::CreateRaw(this, &FGameDepotUEModule::RegisterMenus));

    RunInitializationCheck();
    UE_LOG(LogGameDepotUE, Display, TEXT("GameDepot UE loaded. MockMode=%s Project=%s"), bMockMode ? TEXT("true") : TEXT("false"), *ProjectRoot);
}

void FGameDepotUEModule::ShutdownModule()
{
    UnregisterContentBrowserMenu();
    FGlobalTabmanager::Get()->UnregisterNomadTabSpawner(GameDepotStatusTabName);
    FGlobalTabmanager::Get()->UnregisterNomadTabSpawner(GameDepotConfigTabName);
    if (UToolMenus::IsToolMenuUIEnabled())
    {
        UToolMenus::UnRegisterStartupCallback(this);
        UToolMenus::UnregisterOwner(this);
    }
    ShutdownDaemon();
}

void FGameDepotUEModule::LoadSettings()
{
    FString ConfiguredGameDepotExe;
    FString ConfiguredDaemonExe;

    auto ApplyConfigFile = [&](const FString& IniPath)
    {
        if (!FPaths::FileExists(IniPath))
        {
            return;
        }

        FConfigFile File;
        File.Read(IniPath);

        bool BoolValue = false;
        int32 IntValue = 0;
        FString StringValue;

        if (File.GetBool(TEXT("GameDepotUE"), TEXT("MockMode"), BoolValue)) bMockMode = BoolValue;
        if (File.GetInt(TEXT("GameDepotUE"), TEXT("MaxMockAssets"), IntValue)) MaxMockAssets = IntValue;
        if (File.GetString(TEXT("GameDepotUE"), TEXT("GameDepotExecutable"), StringValue) && !StringValue.TrimStartAndEnd().IsEmpty()) ConfiguredGameDepotExe = StringValue;
        if (File.GetString(TEXT("GameDepotUE"), TEXT("GameDepotDaemonExecutable"), StringValue) && !StringValue.TrimStartAndEnd().IsEmpty()) ConfiguredDaemonExe = StringValue;
        if (File.GetBool(TEXT("GameDepotUE"), TEXT("AutoStartDaemon"), BoolValue)) bAutoStartDaemon = BoolValue;
        if (File.GetBool(TEXT("GameDepotUE"), TEXT("AutoShutdownDaemon"), BoolValue)) bAutoShutdownDaemon = BoolValue;
        if (File.GetString(TEXT("GameDepotUE"), TEXT("DaemonListenAddress"), StringValue) && !StringValue.TrimStartAndEnd().IsEmpty()) DaemonListenAddress = StringValue;
    };

    if (GConfig)
    {
        GConfig->GetBool(TEXT("GameDepotUE"), TEXT("MockMode"), bMockMode, GGameIni);
        GConfig->GetInt(TEXT("GameDepotUE"), TEXT("MaxMockAssets"), MaxMockAssets, GGameIni);
        GConfig->GetString(TEXT("GameDepotUE"), TEXT("GameDepotExecutable"), ConfiguredGameDepotExe, GGameIni);
        GConfig->GetString(TEXT("GameDepotUE"), TEXT("GameDepotDaemonExecutable"), ConfiguredDaemonExe, GGameIni);
        GConfig->GetBool(TEXT("GameDepotUE"), TEXT("AutoStartDaemon"), bAutoStartDaemon, GGameIni);
        GConfig->GetBool(TEXT("GameDepotUE"), TEXT("AutoShutdownDaemon"), bAutoShutdownDaemon, GGameIni);
        GConfig->GetString(TEXT("GameDepotUE"), TEXT("DaemonListenAddress"), DaemonListenAddress, GGameIni);
    }

    // GameDepot writes Config/DefaultGameDepotUE.ini during `gamedepot init`.
    // This file is not part of Unreal's GGameIni chain, so read it explicitly.
    ApplyConfigFile(FPaths::Combine(ProjectRoot, TEXT("Config"), TEXT("DefaultGameDepotUE.ini")));

    MaxMockAssets = FMath::Clamp(MaxMockAssets, 20, 5000);
    if (DaemonListenAddress.IsEmpty()) DaemonListenAddress = TEXT("127.0.0.1:0");

    GameDepotExe = ResolveGameDepotExecutable(ConfiguredGameDepotExe);

    if (GameDepotExe.IsEmpty())
    {
        TArray<FString> ProjectRelativeCandidates;
        ProjectRelativeCandidates.Add(FPaths::ConvertRelativePathToFull(FPaths::Combine(ProjectRoot, TEXT(".."), TEXT("GameDepot"), TEXT("gamedepot.exe"))));
        ProjectRelativeCandidates.Add(FPaths::ConvertRelativePathToFull(FPaths::Combine(ProjectRoot, TEXT(".."), TEXT("gamedepot.exe"))));
        ProjectRelativeCandidates.Add(FPaths::ConvertRelativePathToFull(FPaths::Combine(ProjectRoot, TEXT("gamedepot.exe"))));
        for (const FString& Candidate : ProjectRelativeCandidates)
        {
            if (FPaths::FileExists(Candidate))
            {
                GameDepotExe = Candidate;
                break;
            }
        }
    }

    GameDepotDaemonExe = ResolveGameDepotExecutable(ConfiguredDaemonExe);
    if (GameDepotDaemonExe.IsEmpty())
    {
        GameDepotDaemonExe = GameDepotExe;
    }

    UE_LOG(LogGameDepotUE, Display, TEXT("Resolved GameDepot executable: %s"), GameDepotExe.IsEmpty() ? TEXT("<not found>") : *GameDepotExe);
}

void FGameDepotUEModule::RegisterMenus()
{
    FToolMenuOwnerScoped OwnerScoped(this);
    if (UToolMenu* Menu = UToolMenus::Get()->ExtendMenu("LevelEditor.MainMenu.Tools"))
    {
        FToolMenuSection& Section = Menu->FindOrAddSection("GameDepot");
        Section.Label = LOCTEXT("GameDepotSection", "GameDepot");
        Section.AddMenuEntry("GameDepotOpenStatus", LOCTEXT("OpenStatus", "Changes"), LOCTEXT("OpenStatusTooltip", "Open local changes and conflict list."), FSlateIcon(FAppStyle::GetAppStyleSetName(), "LevelEditor.Tabs.ContentBrowser"), FUIAction(FExecuteAction::CreateRaw(this, &FGameDepotUEModule::OpenStatusTab)));
        Section.AddMenuEntry("GameDepotOpenConfig", LOCTEXT("OpenConfig", "Configuration Manager"), LOCTEXT("OpenConfigTooltip", "Configure OSS/blob storage and rules."), FSlateIcon(FAppStyle::GetAppStyleSetName(), "Icons.Settings"), FUIAction(FExecuteAction::CreateRaw(this, &FGameDepotUEModule::OpenConfigTab)));
        Section.AddMenuEntry("GameDepotInitCheck", LOCTEXT("InitCheck", "Run Initialization Check"), LOCTEXT("InitCheckTooltip", "Check or initialize GameDepot for this project."), FSlateIcon(FAppStyle::GetAppStyleSetName(), "Icons.Warning"), FUIAction(FExecuteAction::CreateRaw(this, &FGameDepotUEModule::PromptInitializeIfNeeded)));
        Section.AddMenuEntry("GameDepotRefresh", LOCTEXT("RefreshStatus", "Refresh Status"), LOCTEXT("RefreshStatusTooltip", "Refresh GameDepot status."), FSlateIcon(FAppStyle::GetAppStyleSetName(), "Icons.Refresh"), FUIAction(FExecuteAction::CreateRaw(this, &FGameDepotUEModule::RefreshData)));
    }

    if (UToolMenu* Toolbar = UToolMenus::Get()->ExtendMenu("LevelEditor.LevelEditorToolBar.User"))
    {
        FToolMenuSection& Section = Toolbar->FindOrAddSection("GameDepot");
        Section.AddEntry(FToolMenuEntry::InitToolBarButton("GameDepotUpdateToolbar", FUIAction(FExecuteAction::CreateRaw(this, &FGameDepotUEModule::Update)), LOCTEXT("ToolbarUpdate", "Update"), LOCTEXT("ToolbarUpdateTooltip", "Update Content assets through GameDepot. Conflicts will open the resolver."), FSlateIcon(FAppStyle::GetAppStyleSetName(), "Icons.Download")));
        Section.AddEntry(FToolMenuEntry::InitToolBarButton("GameDepotPublishToolbar", FUIAction(FExecuteAction::CreateRaw(this, &FGameDepotUEModule::Publish)), LOCTEXT("ToolbarPublish", "Publish"), LOCTEXT("ToolbarPublishTooltip", "Publish local Content changes through GameDepot. Conflicts will open the resolver."), FSlateIcon(FAppStyle::GetAppStyleSetName(), "Icons.Upload")));
    }
}

void FGameDepotUEModule::RegisterStatusTab()
{
    FGlobalTabmanager::Get()->RegisterNomadTabSpawner(GameDepotStatusTabName, FOnSpawnTab::CreateRaw(this, &FGameDepotUEModule::SpawnStatusTab))
        .SetDisplayName(LOCTEXT("StatusTabTitle", "GameDepot Changes"))
        .SetTooltipText(LOCTEXT("StatusTabTooltip", "Local Content changes and conflicts."))
        .SetIcon(FSlateIcon(FAppStyle::GetAppStyleSetName(), "LevelEditor.Tabs.ContentBrowser"));
}

void FGameDepotUEModule::RegisterConfigTab()
{
    FGlobalTabmanager::Get()->RegisterNomadTabSpawner(GameDepotConfigTabName, FOnSpawnTab::CreateRaw(this, &FGameDepotUEModule::SpawnConfigTab))
        .SetDisplayName(LOCTEXT("ConfigTabTitle", "GameDepot Config"))
        .SetTooltipText(LOCTEXT("ConfigTabTooltip", "Configuration manager for OSS/blob storage and rules."))
        .SetIcon(FSlateIcon(FAppStyle::GetAppStyleSetName(), "Icons.Settings"));
}

void FGameDepotUEModule::RegisterContentBrowserMenu()
{
    FContentBrowserModule& ContentBrowserModule = FModuleManager::LoadModuleChecked<FContentBrowserModule>(TEXT("ContentBrowser"));
    TArray<FContentBrowserMenuExtender_SelectedAssets>& Extenders = ContentBrowserModule.GetAllAssetViewContextMenuExtenders();
    Extenders.Add(FContentBrowserMenuExtender_SelectedAssets::CreateRaw(this, &FGameDepotUEModule::OnExtendContentBrowserAssetSelectionMenu));
    ContentBrowserExtenderDelegateHandle = Extenders.Last().GetHandle();
}

void FGameDepotUEModule::UnregisterContentBrowserMenu()
{
    if (!ContentBrowserExtenderDelegateHandle.IsValid() || !FModuleManager::Get().IsModuleLoaded(TEXT("ContentBrowser"))) return;
    FContentBrowserModule& ContentBrowserModule = FModuleManager::GetModuleChecked<FContentBrowserModule>(TEXT("ContentBrowser"));
    TArray<FContentBrowserMenuExtender_SelectedAssets>& Extenders = ContentBrowserModule.GetAllAssetViewContextMenuExtenders();
    Extenders.RemoveAll([this](const FContentBrowserMenuExtender_SelectedAssets& Delegate) { return Delegate.GetHandle() == ContentBrowserExtenderDelegateHandle; });
    ContentBrowserExtenderDelegateHandle.Reset();
}

TSharedRef<SDockTab> FGameDepotUEModule::SpawnStatusTab(const FSpawnTabArgs& Args)
{
    if (!Provider.IsValid()) Provider = MakeShared<FGameDepotMockStatusProvider>();
    if (Provider->GetRows().Num() == 0) RefreshData();
    TSharedRef<SGameDepotStatusPanel> Panel = SNew(SGameDepotStatusPanel)
        .Provider(Provider)
        .OnRefreshRequested(FSimpleDelegate::CreateRaw(this, &FGameDepotUEModule::RefreshData))
        .OnRuleRequested(FGameDepotRulePathsAction::CreateRaw(this, &FGameDepotUEModule::ApplyRuleToPaths))
        .OnHistoryRequested(FGameDepotPathAction::CreateRaw(this, &FGameDepotUEModule::ShowHistoryForPath))
        .OnRevertRequested(FGameDepotPathsAction::CreateRaw(this, &FGameDepotUEModule::RevertPaths))
        .OnHistoryRestored(FGameDepotPathAction::CreateRaw(this, &FGameDepotUEModule::OnStatusPanelHistoryRestored));
    StatusPanel = Panel;
    return SNew(SDockTab).TabRole(ETabRole::NomadTab)[Panel];
}

TSharedRef<SDockTab> FGameDepotUEModule::SpawnConfigTab(const FSpawnTabArgs& Args)
{
    if (!ConfigManager.IsValid()) ConfigManager = MakeShared<FGameDepotConfigManager>();
    TSharedRef<SGameDepotConfigPanel> Panel = SNew(SGameDepotConfigPanel).ConfigManager(ConfigManager).OnConfigSaved(FSimpleDelegate::CreateRaw(this, &FGameDepotUEModule::OnConfigSaved));
    ConfigPanel = Panel;
    if (!bMockMode) PullConfigFromDaemon(true);
    return SNew(SDockTab).TabRole(ETabRole::NomadTab)[Panel];
}

void FGameDepotUEModule::OpenStatusTab() { FGlobalTabmanager::Get()->TryInvokeTab(GameDepotStatusTabName); }
void FGameDepotUEModule::OpenConfigTab() { FGlobalTabmanager::Get()->TryInvokeTab(GameDepotConfigTabName); }

void FGameDepotUEModule::RunInitializationCheck()
{
    if (bMockMode)
    {
        ConfigManager->Load();
        if (!ConfigManager->IsInitialized()) PromptInitializeIfNeeded();
        else { Provider->RebuildFromAssetRegistry(MaxMockAssets); SetToolbarState(EGameDepotToolbarState::PendingUpdate, TEXT("Mock config loaded.")); }
        return;
    }
    const bool bConfigExists = FPaths::FileExists(FPaths::Combine(ProjectRoot, TEXT(".gamedepot/config.yaml")));
    if (!bConfigExists)
    {
        SetToolbarState(EGameDepotToolbarState::PendingUpdate, TEXT("GameDepot is not initialized."));
        PromptInitializeIfNeeded();
        return;
    }
    if (EnsureDaemon())
    {
        PullConfigFromDaemon(false);
        RefreshOverview();
        RefreshData();
    }
}

void FGameDepotUEModule::PromptInitializeIfNeeded()
{
    if (bMockMode)
    {
        if (ConfigManager.IsValid() && ConfigManager->IsInitialized()) { OpenConfigTab(); return; }
        const EAppReturnType::Type Result = FMessageDialog::Open(EAppMsgType::YesNoCancel, LOCTEXT("InitDialogTextMock", "GameDepot mock UI is not initialized for this project.\n\nYes: create mock config.\nNo: open Configuration Manager.\nCancel: skip."));
        if (Result == EAppReturnType::Yes) InitializeWorkspace();
        else if (Result == EAppReturnType::No) OpenConfigTab();
        return;
    }
    if (FPaths::FileExists(FPaths::Combine(ProjectRoot, TEXT(".gamedepot/config.yaml")))) { OpenConfigTab(); return; }
    const EAppReturnType::Type Result = FMessageDialog::Open(EAppMsgType::YesNoCancel, LOCTEXT("InitDialogText", "GameDepot is not initialized for this project.\n\nYes: initialize now.\nNo: open Configuration Manager.\nCancel: skip for this session."));
    if (Result == EAppReturnType::Yes) InitializeWorkspace();
    else if (Result == EAppReturnType::No) OpenConfigTab();
}

void FGameDepotUEModule::InitializeWorkspace()
{
    if (bMockMode)
    {
        FString Error;
        if (ConfigManager->InitializeDefault(Error)) { SetToolbarState(EGameDepotToolbarState::PendingUpdate, TEXT("Mock workspace initialized.")); Notify(TEXT("GameDepot mock workspace initialized.")); OpenConfigTab(); }
        else { SetToolbarState(EGameDepotToolbarState::Error, Error); Notify(Error); }
        return;
    }
    if (GameDepotExe.IsEmpty() || !FPaths::FileExists(GameDepotExe))
    {
        SetToolbarState(EGameDepotToolbarState::Error, TEXT("gamedepot.exe not found in PATH or project fallback paths. Set GameDepotExecutable in DefaultGameDepotUE.ini."));
        Notify(ToolbarStateMessage);
        return;
    }
    int32 Code = -1; FString Out, Err;
    const FString ProjectName = FPaths::GetBaseFilename(FPaths::GetProjectFilePath()).IsEmpty() ? FPaths::GetBaseFilename(ProjectRoot) : FPaths::GetBaseFilename(FPaths::GetProjectFilePath());
    const FString Args = FString::Printf(TEXT("init --project \"%s\" --template ue5"), *ProjectName);
    if (RunGameDepotCLI(Args, Code, Out, Err))
    {
        SetToolbarState(EGameDepotToolbarState::PendingUpdate, TEXT("GameDepot initialized. Starting daemon..."));
        Notify(TEXT("GameDepot initialized."));
        EnsureDaemon();
        PullConfigFromDaemon(true);
        RefreshData();
    }
    else
    {
        SetToolbarState(EGameDepotToolbarState::Error, Err.IsEmpty() ? Out : Err);
        Notify(ToolbarStateMessage);
    }
}

void FGameDepotUEModule::OnConfigSaved()
{
    if (!ConfigManager.IsValid()) return;
    ConfigManager->Load();
    if (bMockMode)
    {
        SetToolbarState(EGameDepotToolbarState::PendingUpdate, TEXT("Mock config changed. Update or publish is pending."));
        Notify(TEXT("GameDepot mock config saved."));
        return;
    }
    PushConfigToDaemon(ConfigManager->GetSnapshot());
}

void FGameDepotUEModule::RefreshData()
{
    if (!Provider.IsValid()) Provider = MakeShared<FGameDepotMockStatusProvider>();
    if (bMockMode)
    {
        Provider->RebuildFromAssetRegistry(MaxMockAssets);
        if (TSharedPtr<SGameDepotStatusPanel> Panel = StatusPanel.Pin()) Panel->RefreshRows();
        SetToolbarState(EGameDepotToolbarState::PendingUpdate, TEXT("Mock status refreshed."));
        return;
    }
    if (!EnsureDaemon()) return;
    RequestJSON(TEXT("POST"), TEXT("/api/ue/v1/assets/changes"), TEXT("{\"exact_hash\":false,\"limit\":500}"), [this](bool bOK, const FString& Text)
    {
        if (!bOK) { SetToolbarState(EGameDepotToolbarState::Error, Text); Notify(TEXT("Refresh changes failed.")); return; }
        FString Error;
        if (!Provider->ReplaceRowsFromDaemonJSON(Text, Error)) { SetToolbarState(EGameDepotToolbarState::Error, Error); Notify(Error); return; }
        if (TSharedPtr<SGameDepotStatusPanel> Panel = StatusPanel.Pin()) Panel->RefreshRows();
        RefreshOverview();
    });
}

void FGameDepotUEModule::Publish()
{
    if (bMockMode)
    {
        SetToolbarState(EGameDepotToolbarState::OK, TEXT("Mock publish succeeded."));
        Notify(ToolbarStateMessage);
        return;
    }
    if (!EnsureDaemon()) return;

    FString Message;
    if (!PromptPublishMessage(Message))
    {
        return;
    }
    if (!SaveDirtyContentPackages(TEXT("Publish")))
    {
        Notify(TEXT("Publish cancelled: assets were not saved."));
        return;
    }

    const FString Body = FString::Printf(TEXT("{\"message\":%s,\"async\":true}"), *QuoteJSON(Message));
    StartGameDepotOperation(TEXT("Publish"), TEXT("/api/ue/v1/publish"), Body);
}

void FGameDepotUEModule::Update()
{
    if (bMockMode)
    {
        SetToolbarState(EGameDepotToolbarState::OK, TEXT("Mock update succeeded."));
        Notify(ToolbarStateMessage);
        return;
    }
    if (!EnsureDaemon()) return;

    if (!SaveDirtyContentPackages(TEXT("Update")))
    {
        Notify(TEXT("Update cancelled: assets were not saved."));
        return;
    }

    StartGameDepotOperation(TEXT("Update"), TEXT("/api/ue/v1/update"), TEXT("{\"mode\":\"normal\",\"async\":true}"));
}

void FGameDepotUEModule::ApplyRuleToPaths(const TArray<FString>& Paths, const FString& Mode)
{
    if (Paths.Num() == 0) return;
    if (bMockMode)
    {
        Provider->SetRuleForDepotPaths(Paths, Mode);
        if (TSharedPtr<SGameDepotStatusPanel> Panel = StatusPanel.Pin()) Panel->RefreshRows();
        SetToolbarState(EGameDepotToolbarState::PendingUpdate, TEXT("Mock rule changed."));
        return;
    }
    if (!EnsureDaemon()) return;
    const FString Kind = Mode == TEXT("git") ? TEXT("manual_git") : (Mode == TEXT("ignore") ? TEXT("manual_ignore") : TEXT("manual_blob"));
    const FString Body = PathsToJSONBody(Paths, FString::Printf(TEXT("\"mode\":%s,\"kind\":%s,\"scope\":\"exact\""), *QuoteJSON(Mode), *QuoteJSON(Kind)));
    RequestJSON(TEXT("POST"), TEXT("/api/ue/v1/rules/upsert"), Body, [this, Mode](bool bOK, const FString& Text)
    {
        if (bOK) { SetToolbarState(EGameDepotToolbarState::PendingUpdate, FString::Printf(TEXT("Rule changed to %s. Publish is pending."), *Mode)); RefreshData(); }
        else { SetToolbarState(EGameDepotToolbarState::Error, Text); Notify(TEXT("Set rule failed.")); }
    });
}

void FGameDepotUEModule::ShowHistoryForPath(const FString& DepotPath)
{
    if (DepotPath.IsEmpty()) return;
    if (bMockMode)
    {
        TSharedRef<SWindow> Window = SNew(SWindow).Title(FText::FromString(FString::Printf(TEXT("GameDepot History - %s"), *DepotPath))).ClientSize(FVector2D(860, 520)).SupportsMaximize(false).SupportsMinimize(false);
        Window->SetContent(SNew(SGameDepotHistoryDialog).DepotPath(DepotPath).Provider(Provider).ParentWindow(TWeakPtr<SWindow>(Window)).OnRestored(FSimpleDelegate::CreateLambda([this, DepotPath]() { OnStatusPanelHistoryRestored(DepotPath); })));
        FSlateApplication::Get().AddWindow(Window);
        return;
    }
    if (!EnsureDaemon()) return;
    const FString Body = FString::Printf(TEXT("{\"path\":%s}"), *QuoteJSON(DepotPath));
    RequestJSON(TEXT("POST"), TEXT("/api/ue/v1/assets/history"), Body, [this, DepotPath](bool bOK, const FString& Text)
    {
        if (!bOK) { SetToolbarState(EGameDepotToolbarState::Error, Text); Notify(TEXT("History query failed.")); return; }
        FString Error;
        if (!Provider->SetHistoryFromDaemonJSON(DepotPath, Text, Error)) { SetToolbarState(EGameDepotToolbarState::Error, Error); Notify(Error); return; }
        TSharedRef<SWindow> Window = SNew(SWindow).Title(FText::FromString(FString::Printf(TEXT("GameDepot History - %s"), *DepotPath))).ClientSize(FVector2D(860, 520)).SupportsMaximize(false).SupportsMinimize(false);
        Window->SetContent(SNew(SGameDepotHistoryDialog).DepotPath(DepotPath).Provider(Provider).ParentWindow(TWeakPtr<SWindow>(Window)).OnRestoreVersion(FGameDepotRestoreVersionAction::CreateRaw(this, &FGameDepotUEModule::RestoreHistoryVersion)).OnRestored(FSimpleDelegate::CreateLambda([this, DepotPath]() { OnStatusPanelHistoryRestored(DepotPath); })));
        FSlateApplication::Get().AddWindow(Window);
    });
}

void FGameDepotUEModule::RestoreHistoryVersion(const FString& DepotPath, const FGameDepotHistoryEntry& Entry)
{
    if (bMockMode)
    {
        Provider->RestoreDepotPathToHistory(DepotPath, Entry);
        OnStatusPanelHistoryRestored(DepotPath);
        return;
    }
    if (!EnsureDaemon()) return;
    const FString Body = FString::Printf(TEXT("{\"path\":%s,\"commit\":%s,\"force\":true}"), *QuoteJSON(DepotPath), *QuoteJSON(Entry.CommitId));
    StartTaskEndpoint(TEXT("/api/ue/v1/assets/restore"), Body, TEXT("Restore history version"));
}

void FGameDepotUEModule::RevertPaths(const TArray<FString>& Paths)
{
    if (Paths.Num() == 0) return;
    if (bMockMode)
    {
        Provider->RevertUncommittedChanges(Paths);
        if (TSharedPtr<SGameDepotStatusPanel> Panel = StatusPanel.Pin()) Panel->RefreshRows();
        SetToolbarState(EGameDepotToolbarState::OK, TEXT("Mock revert completed."));
        return;
    }
    if (!EnsureDaemon()) return;
    RequestJSON(TEXT("POST"), TEXT("/api/ue/v1/assets/revert"), PathsToJSONBody(Paths, TEXT("\"force\":true")), [this](bool bOK, const FString& Text)
    {
        if (bOK) { SetToolbarState(EGameDepotToolbarState::OK, TEXT("Revert completed.")); RefreshData(); }
        else { SetToolbarState(EGameDepotToolbarState::Error, Text); Notify(TEXT("Revert failed.")); }
    });
}

void FGameDepotUEModule::OnStatusPanelHistoryRestored(const FString& DepotPath)
{
    SetToolbarState(EGameDepotToolbarState::PendingUpdate, FString::Printf(TEXT("History version restored for %s. Publish is pending."), *DepotPath));
    RefreshData();
}

void FGameDepotUEModule::SetToolbarState(EGameDepotToolbarState NewState, const FString& Message)
{
    ToolbarState = NewState;
    ToolbarStateMessage = Message;
    if (UToolMenus::IsToolMenuUIEnabled()) UToolMenus::Get()->RefreshAllWidgets();
}

TSharedRef<FExtender> FGameDepotUEModule::OnExtendContentBrowserAssetSelectionMenu(const TArray<FAssetData>& SelectedAssets)
{
    TSharedRef<FExtender> Extender = MakeShared<FExtender>();
    if (SelectedAssets.Num() > 0)
    {
        Extender->AddMenuExtension("GetAssetActions", EExtensionHook::After, nullptr, FMenuExtensionDelegate::CreateRaw(this, &FGameDepotUEModule::AddContentBrowserMenu, SelectedAssets));
    }
    return Extender;
}

void FGameDepotUEModule::AddContentBrowserMenu(FMenuBuilder& MenuBuilder, TArray<FAssetData> SelectedAssets)
{
    MenuBuilder.BeginSection("GameDepot", LOCTEXT("GameDepotMenuHeading", "GameDepot"));
    MenuBuilder.AddMenuEntry(FUIAction(FExecuteAction::CreateLambda([this, SelectedAssets]() { ShowSelectedInStatus(SelectedAssets); })), MakeGameDepotMenuLabel(LOCTEXT("ShowInStatus", "Show in GameDepot Changes")), NAME_None, LOCTEXT("ShowInStatusTooltip", "Open GameDepot changes tab." ));
    MenuBuilder.AddMenuEntry(FUIAction(FExecuteAction::CreateLambda([this, SelectedAssets]() { ShowSelectedHistory(SelectedAssets); })), MakeGameDepotMenuLabel(LOCTEXT("ShowHistory", "Show History Versions...")), NAME_None, LOCTEXT("ShowHistoryTooltip", "Show Git/OSS history versions."));
    MenuBuilder.AddMenuEntry(FUIAction(FExecuteAction::CreateLambda([this, SelectedAssets]() { RevertSelectedUncommittedChanges(SelectedAssets); })), MakeGameDepotMenuLabel(LOCTEXT("RevertUncommitted", "Revert Local Changes")), NAME_None, LOCTEXT("RevertUncommittedTooltip", "Discard local unpublished changes."));
    MenuBuilder.EndSection();
}

void FGameDepotUEModule::ShowSelectedInStatus(TArray<FAssetData> SelectedAssets)
{
    const TArray<FString> Paths = AssetDataArrayToDepotPaths(SelectedAssets);
    if (bMockMode)
    {
        Provider->ClearSelectionMarks(); Provider->AddOrUpdateAssets(SelectedAssets, true); OpenStatusTab();
        if (TSharedPtr<SGameDepotStatusPanel> Panel = StatusPanel.Pin()) { Panel->RefreshRows(); if (Paths.Num() > 0) Panel->FocusPath(Paths[0]); }
        return;
    }
    OpenStatusTab();
    RefreshData();
}

void FGameDepotUEModule::SetSelectedRule(TArray<FAssetData> SelectedAssets, FString Mode) { ApplyRuleToPaths(AssetDataArrayToDepotPaths(SelectedAssets), Mode); }
void FGameDepotUEModule::ShowSelectedHistory(TArray<FAssetData> SelectedAssets) { const TArray<FString> Paths = AssetDataArrayToDepotPaths(SelectedAssets); if (Paths.Num() > 0) ShowHistoryForPath(Paths[0]); }
void FGameDepotUEModule::RevertSelectedUncommittedChanges(TArray<FAssetData> SelectedAssets) { RevertPaths(AssetDataArrayToDepotPaths(SelectedAssets)); }

TArray<FString> FGameDepotUEModule::AssetDataArrayToDepotPaths(const TArray<FAssetData>& SelectedAssets) const
{
    TArray<FString> Paths;
    for (const FAssetData& Asset : SelectedAssets)
    {
        Paths.AddUnique(FGameDepotMockStatusProvider::AssetDataToDepotPath(Asset));
    }
    return Paths;
}

void FGameDepotUEModule::Notify(const FString& Message) const
{
    FNotificationInfo Info(FText::FromString(Message));
    Info.ExpireDuration = 3.0f;
    Info.bUseLargeFont = false;
    FSlateNotificationManager::Get().AddNotification(Info);
    UE_LOG(LogGameDepotUE, Display, TEXT("%s"), *Message);
}

bool FGameDepotUEModule::RunGameDepotCLI(const FString& Args, int32& OutCode, FString& OutStdOut, FString& OutStdErr) const
{
    OutCode = -1; OutStdOut.Reset(); OutStdErr.Reset();
    if (GameDepotExe.IsEmpty() || !FPaths::FileExists(GameDepotExe))
    {
        OutStdErr = TEXT("GameDepotExecutable is missing: ") + GameDepotExe;
        return false;
    }
    const bool bOK = FPlatformProcess::ExecProcess(*GameDepotExe, *Args, &OutCode, &OutStdOut, &OutStdErr);
    return bOK && OutCode == 0;
}

bool FGameDepotUEModule::EnsureDaemon()
{
    if (bMockMode) return true;
    if (ReadRuntimeFile()) return true;
    return bAutoStartDaemon && LaunchDaemon();
}

bool FGameDepotUEModule::ReadRuntimeFile()
{
    FString Text;
    if (!FFileHelper::LoadFileToString(Text, *GetRuntimeFile())) return false;
    TSharedPtr<FJsonObject> Obj;
    if (!ParseJsonObject(Text, Obj)) return false;
    Obj->TryGetStringField(TEXT("addr"), DaemonAddress);
    Obj->TryGetStringField(TEXT("token"), DaemonToken);
    return !DaemonAddress.IsEmpty();
}

bool FGameDepotUEModule::LaunchDaemon()
{
    IFileManager::Get().Delete(*GetRuntimeFile(), false, true, true);
    const FString Exe = FPaths::FileExists(GameDepotDaemonExe) ? GameDepotDaemonExe : GameDepotExe;
    if (Exe.IsEmpty() || !FPaths::FileExists(Exe))
    {
        SetToolbarState(EGameDepotToolbarState::Error, TEXT("gamedepot.exe not found in PATH or project fallback paths. Set GameDepotExecutable in DefaultGameDepotUE.ini."));
        Notify(ToolbarStateMessage);
        return false;
    }
    const FString Args = FString::Printf(TEXT("daemon --root \"%s\" --addr %s --started-by UnrealEditor"), *ProjectRoot, *DaemonListenAddress);
    uint32 PID = 0;
    DaemonProcessHandle = FPlatformProcess::CreateProc(*Exe, *Args, true, true, true, &PID, 0, nullptr, nullptr);
    bStartedDaemon = DaemonProcessHandle.IsValid();
    for (int32 i = 0; i < 50; ++i)
    {
        FPlatformProcess::Sleep(0.1f);
        if (ReadRuntimeFile()) { Notify(TEXT("GameDepot daemon connected.")); return true; }
    }
    SetToolbarState(EGameDepotToolbarState::Error, TEXT("GameDepot daemon startup timed out."));
    Notify(ToolbarStateMessage);
    return false;
}

void FGameDepotUEModule::ShutdownDaemon()
{
    if (!bMockMode && bAutoShutdownDaemon && !DaemonAddress.IsEmpty())
    {
        RequestJSON(TEXT("POST"), TEXT("/api/ue/v1/admin/shutdown"), TEXT("{}"), [](bool, const FString&) {});
    }
    if (bStartedDaemon && DaemonProcessHandle.IsValid()) FPlatformProcess::CloseProc(DaemonProcessHandle);
}

FString FGameDepotUEModule::GetRuntimeFile() const { return FPaths::Combine(ProjectRoot, TEXT(".gamedepot/runtime/daemon.json")); }
FString FGameDepotUEModule::GetURL(const FString& Path) const { return FString::Printf(TEXT("http://%s%s"), *DaemonAddress, *Path); }
FString FGameDepotUEModule::GetAPILogFile() const { return FPaths::ProjectSavedDir() / TEXT("Logs/GameDepotUE_API.log"); }

void FGameDepotUEModule::LogJSON(const FString& Title, const FString& Text) const
{
    const FString LogFile = GetAPILogFile();
    IFileManager::Get().MakeDirectory(*FPaths::GetPath(LogFile), true);
    const FString Line = FString::Printf(TEXT("\n[%s] %s\n%s\n"), *FDateTime::Now().ToString(), *Title, *Text);
    FFileHelper::SaveStringToFile(Line, *LogFile, FFileHelper::EEncodingOptions::AutoDetect, &IFileManager::Get(), FILEWRITE_Append);
    UE_LOG(LogGameDepotUE, Display, TEXT("%s: %s"), *Title, *Text);
}

void FGameDepotUEModule::RequestJSON(const FString& Method, const FString& Path, const FString& Body, TFunction<void(bool, const FString&)> Callback)
{
    RequestJSONInternal(Method, Path, Body, MoveTemp(Callback), true);
}

void FGameDepotUEModule::RequestJSONInternal(const FString& Method, const FString& Path, const FString& Body, TFunction<void(bool, const FString&)> Callback, bool bRetryOnNoResponse)
{
    if (DaemonAddress.IsEmpty() && !EnsureDaemon())
    {
        Callback(false, TEXT("daemon_not_ready"));
        return;
    }

    const FString URL = GetURL(Path);
    LogJSON(FString::Printf(TEXT("REQUEST %s %s"), *Method, *Path), Body.IsEmpty() ? TEXT("<empty>") : Body);

    TSharedRef<IHttpRequest, ESPMode::ThreadSafe> Request = FHttpModule::Get().CreateRequest();
    Request->SetURL(URL);
    Request->SetVerb(Method);
    Request->SetHeader(TEXT("Content-Type"), TEXT("application/json"));
    if (!DaemonToken.IsEmpty()) Request->SetHeader(TEXT("Authorization"), TEXT("Bearer ") + DaemonToken);
    if (!Body.IsEmpty()) Request->SetContentAsString(Body);

    TSharedRef<TFunction<void(bool, const FString&)>> CallbackRef = MakeShared<TFunction<void(bool, const FString&)>>(MoveTemp(Callback));
    Request->OnProcessRequestComplete().BindLambda([this, Method, Path, URL, Body, CallbackRef, bRetryOnNoResponse](FHttpRequestPtr Req, FHttpResponsePtr Resp, bool bTransportOK)
    {
        const FString Text = Resp.IsValid() ? Resp->GetContentAsString() : TEXT("<no response>");
        const int32 Code = Resp.IsValid() ? Resp->GetResponseCode() : 0;
        const bool bSuccess = bTransportOK && Resp.IsValid() && Code >= 200 && Code < 300;
        FString Full = FString::Printf(TEXT("Method: %s\nPath: %s\nURL: %s\nHTTPStatus: %d\nResponseBody:\n%s"), *Method, *Path, *URL, Code, *Text);
        LogJSON(FString::Printf(TEXT("RESPONSE %s HTTP %d"), *Path, Code), Full);

        const bool bNoResponse = !bTransportOK || !Resp.IsValid() || Code == 0;
        if (!bSuccess && bNoResponse && bRetryOnNoResponse)
        {
            LogJSON(TEXT("GameDepot daemon reconnect"), TEXT("No HTTP response from cached daemon address. Deleting runtime file and starting a fresh daemon."));
            DaemonAddress.Reset();
            DaemonToken.Reset();
            IFileManager::Get().Delete(*GetRuntimeFile(), false, true, true);
            if (bAutoStartDaemon && LaunchDaemon())
            {
                RequestJSONInternal(Method, Path, Body, MoveTemp(*CallbackRef), false);
                return;
            }
        }

        (*CallbackRef)(bSuccess, bSuccess ? Text : Full);
    });

    if (!Request->ProcessRequest())
    {
        if (bRetryOnNoResponse)
        {
            DaemonAddress.Reset();
            DaemonToken.Reset();
            IFileManager::Get().Delete(*GetRuntimeFile(), false, true, true);
            if (bAutoStartDaemon && LaunchDaemon())
            {
                RequestJSONInternal(Method, Path, Body, MoveTemp(*CallbackRef), false);
                return;
            }
        }
        (*CallbackRef)(false, FString::Printf(TEXT("HTTP request start failed: %s"), *URL));
    }
}

FString FGameDepotUEModule::PathsToJSONBody(const TArray<FString>& Paths, const FString& ExtraFields) const
{
    FString Body = TEXT("{\"paths\":[");
    for (int32 i = 0; i < Paths.Num(); ++i)
    {
        if (i > 0) Body += TEXT(",");
        Body += QuoteJSON(Paths[i]);
    }
    Body += TEXT("]");
    if (!ExtraFields.IsEmpty()) Body += TEXT(",") + ExtraFields;
    Body += TEXT("}");
    return Body;
}

void FGameDepotUEModule::StartTaskEndpoint(const FString& Path, const FString& Body, const FString& FriendlyName)
{
    FNotificationInfo Info(FText::FromString(FriendlyName + TEXT(" started...")));
    Info.bFireAndForget = false;
    Info.bUseThrobber = true;
    ActiveTaskNotification = FSlateNotificationManager::Get().AddNotification(Info);
    if (ActiveTaskNotification.IsValid()) ActiveTaskNotification.Pin()->SetCompletionState(SNotificationItem::CS_Pending);
    RequestJSON(TEXT("POST"), Path, Body, [this, FriendlyName](bool bOK, const FString& Text)
    {
        FString TaskID;
        if (bOK && TryGetStringField(Text, TEXT("task_id"), TaskID)) { PollTask(TaskID, FriendlyName); return; }
        if (ActiveTaskNotification.IsValid()) { ActiveTaskNotification.Pin()->SetText(FText::FromString(FriendlyName + TEXT(" failed to start"))); ActiveTaskNotification.Pin()->SetCompletionState(SNotificationItem::CS_Fail); ActiveTaskNotification.Pin()->ExpireAndFadeout(); }
        SetToolbarState(EGameDepotToolbarState::Error, Text); Notify(FriendlyName + TEXT(" failed to start."));
    });
}

void FGameDepotUEModule::PollTask(const FString& TaskID, const FString& FriendlyName)
{
    RequestJSON(TEXT("GET"), TEXT("/api/ue/v1/tasks/") + TaskID, TEXT(""), [this, TaskID, FriendlyName](bool bOK, const FString& Text)
    {
        if (!bOK) { SetToolbarState(EGameDepotToolbarState::Error, Text); Notify(FriendlyName + TEXT(" task poll failed.")); return; }
        TSharedPtr<FJsonObject> TaskObj;
        if (!TryGetObjectField(Text, TEXT("task"), TaskObj) || !TaskObj.IsValid()) { SetToolbarState(EGameDepotToolbarState::Error, TEXT("Invalid task response.")); return; }
        FString Status, Phase, Message, Error;
        TaskObj->TryGetStringField(TEXT("status"), Status);
        TaskObj->TryGetStringField(TEXT("phase"), Phase);
        TaskObj->TryGetStringField(TEXT("message"), Message);
        TaskObj->TryGetStringField(TEXT("error"), Error);
        if (ActiveTaskNotification.IsValid()) ActiveTaskNotification.Pin()->SetText(FText::FromString(FriendlyName + TEXT(": ") + Phase + TEXT(" ") + Message));
        if (Status == TEXT("succeeded"))
        {
            if (ActiveTaskNotification.IsValid()) { ActiveTaskNotification.Pin()->SetCompletionState(SNotificationItem::CS_Success); ActiveTaskNotification.Pin()->ExpireAndFadeout(); }
            SetToolbarState(EGameDepotToolbarState::OK, FriendlyName + TEXT(" completed.")); Notify(ToolbarStateMessage); RefreshData();
        }
        else if (Status == TEXT("failed"))
        {
            if (ActiveTaskNotification.IsValid()) { ActiveTaskNotification.Pin()->SetCompletionState(SNotificationItem::CS_Fail); ActiveTaskNotification.Pin()->ExpireAndFadeout(); }
            SetToolbarState(EGameDepotToolbarState::Error, Error.IsEmpty() ? Text : Error); Notify(FriendlyName + TEXT(" failed."));
        }
        else
        {
            FTSTicker::GetCoreTicker().AddTicker(FTickerDelegate::CreateLambda([this, TaskID, FriendlyName](float) { PollTask(TaskID, FriendlyName); return false; }), 0.5f);
        }
    });
}

void FGameDepotUEModule::StartGameDepotOperation(const FString& FriendlyName, const FString& Path, const FString& Body)
{
    TSharedRef<SWindow> Window = SNew(SWindow)
        .Title(FText::FromString(TEXT("GameDepot ") + FriendlyName))
        .ClientSize(FVector2D(720.0f, 420.0f))
        .SupportsMaximize(false)
        .SupportsMinimize(false);

    TSharedRef<SGameDepotOperationDialog> Dialog = SNew(SGameDepotOperationDialog).OperationName(TEXT("GameDepot ") + FriendlyName);
    OperationDialog = Dialog;
    Window->SetContent(Dialog);
    FSlateApplication::Get().AddWindow(Window);

    Dialog->AppendLog(FriendlyName + TEXT(" requested."));
    RequestJSON(TEXT("POST"), Path, Body, [this, FriendlyName](bool bOK, const FString& Text)
    {
        FString TaskID;
        if (bOK && TryGetStringField(Text, TEXT("task_id"), TaskID))
        {
            PollGameDepotOperation(TaskID, FriendlyName);
            return;
        }
        if (TSharedPtr<SGameDepotOperationDialog> Dialog = OperationDialog.Pin())
        {
            Dialog->SetFailed(Text);
        }
        SetToolbarState(EGameDepotToolbarState::Error, Text);
        Notify(FriendlyName + TEXT(" failed to start."));
        ShowConflictDialog();
    });
}

void FGameDepotUEModule::PollGameDepotOperation(const FString& TaskID, const FString& FriendlyName)
{
    RequestJSON(TEXT("GET"), TEXT("/api/ue/v1/tasks/") + TaskID, TEXT(""), [this, TaskID, FriendlyName](bool bOK, const FString& Text)
    {
        if (!bOK)
        {
            if (TSharedPtr<SGameDepotOperationDialog> Dialog = OperationDialog.Pin())
            {
                Dialog->SetFailed(Text);
            }
            SetToolbarState(EGameDepotToolbarState::Error, Text);
            Notify(FriendlyName + TEXT(" task poll failed."));
            return;
        }

        TSharedPtr<FJsonObject> TaskObj;
        if (!TryGetObjectField(Text, TEXT("task"), TaskObj) || !TaskObj.IsValid())
        {
            SetToolbarState(EGameDepotToolbarState::Error, TEXT("Invalid task response."));
            return;
        }

        FString Status, Phase, Message, Error;
        TaskObj->TryGetStringField(TEXT("status"), Status);
        TaskObj->TryGetStringField(TEXT("phase"), Phase);
        TaskObj->TryGetStringField(TEXT("message"), Message);
        TaskObj->TryGetStringField(TEXT("error"), Error);

        double CurrentNumber = 0.0;
        double TotalNumber = 0.0;
        double PercentNumber = 0.0;
        TaskObj->TryGetNumberField(TEXT("current"), CurrentNumber);
        TaskObj->TryGetNumberField(TEXT("total"), TotalNumber);
        TaskObj->TryGetNumberField(TEXT("percent"), PercentNumber);

        TArray<FString> Logs;
        const TArray<TSharedPtr<FJsonValue>>* LogValues = nullptr;
        if (TaskObj->TryGetArrayField(TEXT("logs"), LogValues) && LogValues)
        {
            for (const TSharedPtr<FJsonValue>& V : *LogValues)
            {
                Logs.Add(V.IsValid() ? V->AsString() : FString());
            }
        }

        if (TSharedPtr<SGameDepotOperationDialog> Dialog = OperationDialog.Pin())
        {
            Dialog->SetProgress(Phase, Message, static_cast<int32>(CurrentNumber), static_cast<int32>(TotalNumber), static_cast<int32>(PercentNumber));
            Dialog->SetLogs(Logs);
        }

        if (Status == TEXT("succeeded"))
        {
            if (TSharedPtr<SGameDepotOperationDialog> Dialog = OperationDialog.Pin())
            {
                Dialog->SetSucceeded(FriendlyName + TEXT(" completed."));
            }
            SetToolbarState(EGameDepotToolbarState::OK, FriendlyName + TEXT(" completed."));
            Notify(ToolbarStateMessage);
            RefreshContentBrowserAfterDepotChange();
            RefreshData();
            if (FriendlyName.StartsWith(TEXT("Resolve")))
            {
                ShowConflictDialog();
            }
        }
        else if (Status == TEXT("failed"))
        {
            const FString ErrorText = Error.IsEmpty() ? Text : Error;
            if (TSharedPtr<SGameDepotOperationDialog> Dialog = OperationDialog.Pin())
            {
                Dialog->SetFailed(ErrorText);
            }
            SetToolbarState(EGameDepotToolbarState::Error, ErrorText);
            Notify(FriendlyName + TEXT(" needs attention."));
            ShowConflictDialog();
        }
        else
        {
            FTSTicker::GetCoreTicker().AddTicker(FTickerDelegate::CreateLambda([this, TaskID, FriendlyName](float) { PollGameDepotOperation(TaskID, FriendlyName); return false; }), 0.35f);
        }
    });
}

void FGameDepotUEModule::ShowConflictDialog()
{
    RequestJSON(TEXT("GET"), TEXT("/api/ue/v1/conflicts"), TEXT(""), [this](bool bOK, const FString& Text)
    {
        if (!bOK)
        {
            return;
        }
        ShowConflictDialogFromJSON(Text);
    });
}

void FGameDepotUEModule::ShowConflictDialogFromJSON(const FString& JsonText)
{
    TSharedPtr<FJsonObject> Root;
    if (!ParseJsonObject(JsonText, Root) || !Root.IsValid())
    {
        return;
    }
    bool bActive = false;
    Root->TryGetBoolField(TEXT("active"), bActive);
    const TArray<TSharedPtr<FJsonValue>>* ConflictValues = nullptr;
    if (!Root->TryGetArrayField(TEXT("conflicts"), ConflictValues) || !ConflictValues || ConflictValues->Num() == 0)
    {
        return;
    }
    if (!bActive)
    {
        return;
    }

    TArray<FGameDepotConflictItemPtr> Items;
    for (const TSharedPtr<FJsonValue>& V : *ConflictValues)
    {
        const TSharedPtr<FJsonObject> Obj = V.IsValid() ? V->AsObject() : nullptr;
        if (!Obj.IsValid())
        {
            continue;
        }
        FGameDepotConflictItemPtr Item = MakeShared<FGameDepotConflictItem>();
        Obj->TryGetStringField(TEXT("path"), Item->Path);
        Obj->TryGetStringField(TEXT("kind"), Item->Kind);
        Obj->TryGetStringField(TEXT("base_oid"), Item->BaseOID);
        Obj->TryGetStringField(TEXT("local_oid"), Item->LocalOID);
        Obj->TryGetStringField(TEXT("remote_oid"), Item->RemoteOID);
        if (!Item->Path.IsEmpty())
        {
            Items.Add(Item);
        }
    }
    if (Items.Num() == 0)
    {
        return;
    }

    TSharedRef<SWindow> Window = SNew(SWindow)
        .Title(LOCTEXT("ConflictWindowTitle", "GameDepot Conflicts"))
        .ClientSize(FVector2D(920.0f, 520.0f))
        .SupportsMaximize(false)
        .SupportsMinimize(false);
    Window->SetContent(SNew(SGameDepotConflictDialog)
        .Conflicts(Items)
        .OnResolveRequested(FGameDepotResolveConflictAction::CreateRaw(this, &FGameDepotUEModule::ResolveConflict)));
    FSlateApplication::Get().AddWindow(Window);
}

void FGameDepotUEModule::ResolveConflict(const FString& DepotPath, const FString& Decision)
{
    if (!EnsureDaemon()) return;
    const FString Body = FString::Printf(TEXT("{\"path\":%s,\"decision\":%s,\"async\":true}"), *QuoteJSON(DepotPath), *QuoteJSON(Decision));
    StartGameDepotOperation(Decision == TEXT("remote") ? TEXT("Resolve Remote") : TEXT("Resolve Local"), TEXT("/api/ue/v1/conflicts/resolve"), Body);
}

bool FGameDepotUEModule::PromptPublishMessage(FString& OutMessage) const
{
    bool bAccepted = false;
    TSharedPtr<SEditableTextBox> TextBox;
    TSharedRef<SWindow> Window = SNew(SWindow)
        .Title(LOCTEXT("PublishMessageTitle", "GameDepot Publish"))
        .ClientSize(FVector2D(520.0f, 150.0f))
        .SupportsMaximize(false)
        .SupportsMinimize(false);

    Window->SetContent(
        SNew(SBorder)
        .BorderImage(FAppStyle::GetBrush("Brushes.Panel"))
        .Padding(12.0f)
        [
            SNew(SVerticalBox)
            + SVerticalBox::Slot().AutoHeight().Padding(0.0f, 0.0f, 0.0f, 8.0f)
            [
                SNew(STextBlock).Text(LOCTEXT("PublishMessagePrompt", "Publish message"))
            ]
            + SVerticalBox::Slot().AutoHeight().Padding(0.0f, 0.0f, 0.0f, 12.0f)
            [
                SAssignNew(TextBox, SEditableTextBox)
                .Text(FText::FromString(TEXT("Publish from Unreal Editor")))
                .SelectAllTextWhenFocused(true)
            ]
            + SVerticalBox::Slot().AutoHeight().HAlign(HAlign_Right)
            [
                SNew(SHorizontalBox)
                + SHorizontalBox::Slot().AutoWidth().Padding(0.0f, 0.0f, 8.0f, 0.0f)
                [
                    SNew(SButton)
                    .Text(LOCTEXT("CancelPublish", "Cancel"))
                    .OnClicked_Lambda([&Window]() { Window->RequestDestroyWindow(); return FReply::Handled(); })
                ]
                + SHorizontalBox::Slot().AutoWidth()
                [
                    SNew(SButton)
                    .Text(LOCTEXT("StartPublish", "Publish"))
                    .OnClicked_Lambda([&bAccepted, &Window]() { bAccepted = true; Window->RequestDestroyWindow(); return FReply::Handled(); })
                ]
            ]
        ]);

    FSlateApplication::Get().AddModalWindow(Window, TSharedPtr<SWindow>());
    OutMessage = TextBox.IsValid() ? TextBox->GetText().ToString().TrimStartAndEnd() : FString();
    return bAccepted && !OutMessage.IsEmpty();
}

bool FGameDepotUEModule::SaveDirtyContentPackages(const FString& Reason) const
{
    const EAppReturnType::Type Choice = FMessageDialog::Open(EAppMsgType::YesNoCancel, FText::FromString(FString::Printf(TEXT("GameDepot %s needs saved Content files on disk.\n\nSave dirty Content packages now?"), *Reason)));
    if (Choice != EAppReturnType::Yes)
    {
        return false;
    }
    return FEditorFileUtils::SaveDirtyPackages(true, false, true);
}

void FGameDepotUEModule::RefreshContentBrowserAfterDepotChange()
{
    if (!FModuleManager::Get().IsModuleLoaded(TEXT("AssetRegistry")))
    {
        return;
    }
    FAssetRegistryModule& AssetRegistryModule = FModuleManager::LoadModuleChecked<FAssetRegistryModule>(TEXT("AssetRegistry"));
    TArray<FString> PathsToScan;
    PathsToScan.Add(TEXT("/Game"));
    AssetRegistryModule.Get().ScanPathsSynchronous(PathsToScan, true);
}

void FGameDepotUEModule::PullConfigFromDaemon(bool bRefreshPanel)
{
    if (bMockMode || !EnsureDaemon()) return;
    RequestJSON(TEXT("GET"), TEXT("/api/ue/v1/config"), TEXT(""), [this, bRefreshPanel](bool bOK, const FString& Text)
    {
        if (!bOK) { SetToolbarState(EGameDepotToolbarState::Error, Text); return; }
        TSharedPtr<FJsonObject> Root;
        if (!ParseJsonObject(Text, Root)) return;
        FGameDepotConfigSnapshot Snap = ConfigManager.IsValid() ? ConfigManager->GetSnapshot() : FGameDepotConfigSnapshot();
        Snap.bInitialized = true;
        const TSharedPtr<FJsonObject>* OSS = nullptr; if (Root->TryGetObjectField(TEXT("oss"), OSS) && OSS && OSS->IsValid()) { (*OSS)->TryGetStringField(TEXT("provider"), Snap.OSSProvider); (*OSS)->TryGetStringField(TEXT("endpoint"), Snap.OSSEndpoint); (*OSS)->TryGetStringField(TEXT("bucket"), Snap.OSSBucket); (*OSS)->TryGetStringField(TEXT("region"), Snap.OSSRegion); (*OSS)->TryGetStringField(TEXT("blob_prefix"), Snap.OSSPrefix); }
        if (ConfigManager.IsValid()) ConfigManager->SetSnapshot(Snap);
        RequestJSON(TEXT("GET"), TEXT("/api/ue/v1/rules"), TEXT(""), [this, bRefreshPanel, Snap](bool bRulesOK, const FString& RulesText) mutable
        {
            if (bRulesOK)
            {
                TSharedPtr<FJsonObject> RulesRoot;
                if (ParseJsonObject(RulesText, RulesRoot))
                {
                    const TArray<TSharedPtr<FJsonValue>>* Rules = nullptr;
                    if (RulesRoot->TryGetArrayField(TEXT("rules"), Rules) && Rules)
                    {
                        Snap.Rules.Reset();
                        for (const TSharedPtr<FJsonValue>& V : *Rules)
                        {
                            const TSharedPtr<FJsonObject> R = V.IsValid() ? V->AsObject() : nullptr;
                            if (!R.IsValid()) continue;
                            FGameDepotRuleConfig Rule;
                            R->TryGetStringField(TEXT("pattern"), Rule.Pattern);
                            R->TryGetStringField(TEXT("mode"), Rule.Mode);
                            R->TryGetStringField(TEXT("scope"), Rule.Scope);
                            if (Rule.Scope.IsEmpty()) Rule.Scope = TEXT("glob");
                            if (!Rule.Pattern.IsEmpty()) Snap.Rules.Add(Rule);
                        }
                        Snap.RuleText = FGameDepotConfigManager::RulesToText(Snap.Rules);
                        if (ConfigManager.IsValid()) ConfigManager->SetSnapshot(Snap);
                    }
                }
            }
            if (bRefreshPanel) { if (TSharedPtr<SGameDepotConfigPanel> Panel = ConfigPanel.Pin()) Panel->RefreshFromManager(); }
        });
    });
}

void FGameDepotUEModule::PushConfigToDaemon(const FGameDepotConfigSnapshot& Snapshot)
{
    if (!EnsureDaemon()) return;
    const FString Body = FString::Printf(TEXT("{\"oss\":{\"provider\":%s,\"endpoint\":%s,\"bucket\":%s,\"region\":%s,\"blob_prefix\":%s}}"), *QuoteJSON(Snapshot.OSSProvider), *QuoteJSON(Snapshot.OSSEndpoint), *QuoteJSON(Snapshot.OSSBucket), *QuoteJSON(Snapshot.OSSRegion), *QuoteJSON(Snapshot.OSSPrefix));
    RequestJSON(TEXT("POST"), TEXT("/api/ue/v1/config"), Body, [this, Snapshot](bool bOK, const FString& Text)
    {
        if (!bOK) { SetToolbarState(EGameDepotToolbarState::Error, Text); Notify(TEXT("Save config failed.")); return; }
        for (const FGameDepotRuleConfig& Rule : Snapshot.Rules)
        {
            if (Rule.Pattern.TrimStartAndEnd().IsEmpty()) continue;
            const FString RuleBody = FString::Printf(TEXT("{\"pattern\":%s,\"mode\":%s,\"scope\":%s}"), *QuoteJSON(Rule.Pattern), *QuoteJSON(Rule.Mode), *QuoteJSON(Rule.Scope.IsEmpty() ? TEXT("glob") : Rule.Scope));
            RequestJSON(TEXT("POST"), TEXT("/api/ue/v1/rules/upsert"), RuleBody, [](bool, const FString&) {});
        }
        SetToolbarState(EGameDepotToolbarState::PendingUpdate, TEXT("Config saved to daemon. Update or publish is pending."));
        Notify(TEXT("GameDepot config saved."));
        PullConfigFromDaemon(true);
        RefreshData();
    });
}

void FGameDepotUEModule::RefreshOverview()
{
    if (bMockMode || !EnsureDaemon()) return;
    RequestJSON(TEXT("GET"), TEXT("/api/ue/v1/overview"), TEXT(""), [this](bool bOK, const FString& Text)
    {
        if (bOK) ParseAndApplyOverview(Text);
    });
}

void FGameDepotUEModule::ParseAndApplyOverview(const FString& JsonText)
{
    TSharedPtr<FJsonObject> Root;
    if (!ParseJsonObject(JsonText, Root)) return;
    const TSharedPtr<FJsonObject>* Changes = nullptr;
    int32 Modified = 0, NewFiles = 0, Deleted = 0, LocalOnly = 0;
    if (Root->TryGetObjectField(TEXT("changes"), Changes) && Changes && Changes->IsValid())
    {
        double N = 0;
        if ((*Changes)->TryGetNumberField(TEXT("modified"), N)) Modified = static_cast<int32>(N);
        if ((*Changes)->TryGetNumberField(TEXT("new_files"), N)) NewFiles = static_cast<int32>(N);
        if ((*Changes)->TryGetNumberField(TEXT("deleted"), N)) Deleted = static_cast<int32>(N);
        if ((*Changes)->TryGetNumberField(TEXT("local_only"), N)) LocalOnly = static_cast<int32>(N);
    }
    const int32 Pending = Modified + NewFiles + Deleted + LocalOnly;
    if (Pending > 0) SetToolbarState(EGameDepotToolbarState::PendingUpdate, FString::Printf(TEXT("Pending update: %d modified, %d new/local, %d deleted."), Modified, NewFiles + LocalOnly, Deleted));
    else SetToolbarState(EGameDepotToolbarState::OK, TEXT("Workspace is clean."));
}

#undef LOCTEXT_NAMESPACE
IMPLEMENT_MODULE(FGameDepotUEModule, GameDepotUE)
