#include "GameDepotMockStatusProvider.h"

#include "AssetRegistry/AssetRegistryModule.h"
#include "Modules/ModuleManager.h"
#include "Misc/Paths.h"
#include "Misc/Crc.h"
#include "Dom/JsonObject.h"
#include "Serialization/JsonReader.h"
#include "Serialization/JsonSerializer.h"

void FGameDepotMockStatusProvider::RebuildFromAssetRegistry(int32 MaxAssets)
{
    Rows.Reset();
    RowByPath.Reset();

    FAssetRegistryModule& AssetRegistryModule = FModuleManager::LoadModuleChecked<FAssetRegistryModule>(TEXT("AssetRegistry"));
    TArray<FAssetData> Assets;
    AssetRegistryModule.Get().GetAssetsByPath(FName(TEXT("/Game")), Assets, true);

    Assets.Sort([](const FAssetData& A, const FAssetData& B)
    {
        return A.PackageName.LexicalLess(B.PackageName);
    });

    int32 Count = 0;
    for (const FAssetData& Asset : Assets)
    {
        if (Count >= MaxAssets)
        {
            break;
        }
        if (!Asset.IsValid())
        {
            continue;
        }
        UpsertRow(MakeRowForAsset(Asset, false));
        ++Count;
    }

    if (Rows.Num() == 0)
    {
        AddOrUpdateDepotPaths({
            TEXT("Content/Characters/Hero.uasset"),
            TEXT("Content/Maps/Arena.umap"),
            TEXT("Content/Audio/BGM_Battle.wav"),
            TEXT("Content/Data/Items.json"),
            TEXT("Content/Weird/VendorCache.custom")
        }, false);
    }

    AddMockHistoryOnlyRows();
}

void FGameDepotMockStatusProvider::AddOrUpdateAssets(const TArray<FAssetData>& Assets, bool bMarkSelected)
{
    for (const FAssetData& Asset : Assets)
    {
        UpsertRow(MakeRowForAsset(Asset, bMarkSelected));
    }
}

void FGameDepotMockStatusProvider::AddOrUpdateDepotPaths(const TArray<FString>& DepotPaths, bool bMarkSelected)
{
    for (const FString& DepotPath : DepotPaths)
    {
        UpsertRow(MakeRowForDepotPath(DepotPath, bMarkSelected));
    }
}

void FGameDepotMockStatusProvider::SetRuleForDepotPaths(const TArray<FString>& DepotPaths, const FString& Mode)
{
    for (const FString& DepotPath : DepotPaths)
    {
        FGameDepotAssetRowPtr Row = FindByDepotPath(DepotPath);
        if (!Row.IsValid())
        {
            Row = MakeRowForDepotPath(DepotPath, true);
            UpsertRow(Row);
        }

        Row->bSelected = true;
        Row->DesiredRule = Mode;
        Row->Message = FString::Printf(TEXT("Mock rule set to '%s'. This is UI-only and will later call the daemon."), *Mode);

        if (Mode.Equals(TEXT("blob"), ESearchCase::IgnoreCase) || Mode.Equals(TEXT("oss"), ESearchCase::IgnoreCase))
        {
            Row->Storage = Row->bGitTracked ? EGameDepotStorage::OSS : EGameDepotStorage::NewToOSS;
            Row->Sync = Row->bRemoteExists ? EGameDepotSyncState::Synced : EGameDepotSyncState::NewFile;
            Row->Severity = EGameDepotSeverity::Info;
            Row->RemoteState = Row->bRemoteExists ? TEXT("Exists") : TEXT("Pending upload");
        }
        else if (Mode.Equals(TEXT("git"), ESearchCase::IgnoreCase))
        {
            Row->Storage = Row->bGitTracked ? EGameDepotStorage::Git : EGameDepotStorage::NewToGit;
            Row->Sync = Row->bGitTracked ? EGameDepotSyncState::Modified : EGameDepotSyncState::NewFile;
            Row->Severity = EGameDepotSeverity::Info;
            Row->RemoteState = TEXT("N/A");
        }
        else if (Mode.Equals(TEXT("ignore"), ESearchCase::IgnoreCase))
        {
            Row->Storage = EGameDepotStorage::Ignored;
            Row->Sync = EGameDepotSyncState::Ignored;
            Row->Severity = EGameDepotSeverity::Muted;
            Row->RemoteState = TEXT("N/A");
        }
        else
        {
            Row->Storage = EGameDepotStorage::Review;
            Row->Sync = EGameDepotSyncState::ReviewRequired;
            Row->Severity = EGameDepotSeverity::Warning;
        }
    }
}

void FGameDepotMockStatusProvider::ClearSelectionMarks()
{
    for (const FGameDepotAssetRowPtr& Row : Rows)
    {
        if (Row.IsValid())
        {
            Row->bSelected = false;
        }
    }
}

FGameDepotAssetRowPtr FGameDepotMockStatusProvider::FindByDepotPath(const FString& DepotPath) const
{
    if (const FGameDepotAssetRowPtr* Found = RowByPath.Find(DepotPath))
    {
        return *Found;
    }
    return nullptr;
}

int32 FGameDepotMockStatusProvider::CountByStorage(EGameDepotStorage Storage) const
{
    int32 Count = 0;
    for (const FGameDepotAssetRowPtr& Row : Rows)
    {
        if (Row.IsValid() && Row->Storage == Storage)
        {
            ++Count;
        }
    }
    return Count;
}

int32 FGameDepotMockStatusProvider::CountBySync(EGameDepotSyncState Sync) const
{
    int32 Count = 0;
    for (const FGameDepotAssetRowPtr& Row : Rows)
    {
        if (Row.IsValid() && Row->Sync == Sync)
        {
            ++Count;
        }
    }
    return Count;
}

int32 FGameDepotMockStatusProvider::CountBySeverity(EGameDepotSeverity Severity) const
{
    int32 Count = 0;
    for (const FGameDepotAssetRowPtr& Row : Rows)
    {
        if (Row.IsValid() && Row->Severity == Severity)
        {
            ++Count;
        }
    }
    return Count;
}

int32 FGameDepotMockStatusProvider::CountHistoryOnly() const
{
    int32 Count = 0;
    for (const FGameDepotAssetRowPtr& Row : Rows)
    {
        if (Row.IsValid() && Row->bHistoryOnly)
        {
            ++Count;
        }
    }
    return Count;
}

void FGameDepotMockStatusProvider::AddMockHistoryOnlyRows()
{
    UpsertRow(MakeHistoryOnlyRow(
        TEXT("Content/Deleted/OldBoss.uasset"),
        EGameDepotStorage::OSS,
        42ll * 1024ll * 1024ll,
        TEXT("Not in current Content Browser, but found in older commit manifests. Restore from history to bring it back.")));
    UpsertRow(MakeHistoryOnlyRow(
        TEXT("Content/Maps/Prototype_Arena_Old.umap"),
        EGameDepotStorage::OSS,
        118ll * 1024ll * 1024ll,
        TEXT("Map was deleted locally/currently absent. History still contains Git/OSS routed versions.")));
    UpsertRow(MakeHistoryOnlyRow(
        TEXT("Content/Data/DeprecatedItems.json"),
        EGameDepotStorage::Git,
        96ll * 1024ll,
        TEXT("Git-managed data file is absent from current Content, but can be restored from an older commit.")));
}

FString FGameDepotMockStatusProvider::AssetDataToDepotPath(const FAssetData& AssetData)
{
    FString PackageName = AssetData.PackageName.ToString();
    FString Relative = PackageName;
    if (Relative.RemoveFromStart(TEXT("/Game/")))
    {
        return FString::Printf(TEXT("Content/%s.uasset"), *Relative);
    }
    if (Relative == TEXT("/Game"))
    {
        return TEXT("Content/Unknown.uasset");
    }
    Relative.RemoveFromStart(TEXT("/"));
    return Relative + TEXT(".uasset");
}

FString FGameDepotMockStatusProvider::ObjectPathToDepotPath(const FString& ObjectPath)
{
    FString PackagePart = ObjectPath;
    int32 DotIndex = INDEX_NONE;
    if (PackagePart.FindChar(TEXT('.'), DotIndex))
    {
        PackagePart.LeftInline(DotIndex);
    }
    if (PackagePart.StartsWith(TEXT("/Game/")))
    {
        PackagePart.RemoveFromStart(TEXT("/Game/"));
        return FString::Printf(TEXT("Content/%s.uasset"), *PackagePart);
    }
    return PackagePart;
}

FGameDepotAssetRowPtr FGameDepotMockStatusProvider::MakeRowForAsset(const FAssetData& AssetData, bool bMarkSelected) const
{
    FGameDepotAssetRowPtr Row = MakeShared<FGameDepotAssetRow>();
    Row->DepotPath = AssetDataToDepotPath(AssetData);
    Row->PackageName = AssetData.PackageName.ToString();
    Row->AssetName = AssetData.AssetName.ToString();
    Row->ClassName = AssetData.AssetClassPath.GetAssetName().ToString();
    Row->bSelected = bMarkSelected;
    ApplyDeterministicMockState(*Row);
    return Row;
}

FGameDepotAssetRowPtr FGameDepotMockStatusProvider::MakeRowForDepotPath(const FString& DepotPath, bool bMarkSelected) const
{
    FGameDepotAssetRowPtr Row = MakeShared<FGameDepotAssetRow>();
    Row->DepotPath = DepotPath;
    Row->AssetName = FPaths::GetBaseFilename(DepotPath);
    FString NoExt = DepotPath;
    int32 ExtensionDotIndex = INDEX_NONE;
    if (NoExt.FindLastChar(TEXT('.'), ExtensionDotIndex))
    {
        NoExt.LeftInline(ExtensionDotIndex);
    }
    Row->PackageName = TEXT("/Game/") + NoExt;
    Row->PackageName.RemoveFromStart(TEXT("/Game/Content/"));
    Row->PackageName = TEXT("/Game/") + Row->PackageName;
    Row->ClassName = TEXT("Asset");
    Row->bSelected = bMarkSelected;
    ApplyDeterministicMockState(*Row);
    return Row;
}

FGameDepotAssetRowPtr FGameDepotMockStatusProvider::MakeHistoryOnlyRow(const FString& DepotPath, EGameDepotStorage Storage, int64 SizeBytes, const FString& Message) const
{
    FGameDepotAssetRowPtr Row = MakeShared<FGameDepotAssetRow>();
    Row->DepotPath = DepotPath;
    Row->AssetName = FPaths::GetBaseFilename(DepotPath);
    FString NoExt = DepotPath;
    int32 ExtensionDotIndex = INDEX_NONE;
    if (NoExt.FindLastChar(TEXT('.'), ExtensionDotIndex))
    {
        NoExt.LeftInline(ExtensionDotIndex);
    }
    Row->PackageName = TEXT("/Game/") + NoExt;
    Row->PackageName.RemoveFromStart(TEXT("/Game/Content/"));
    Row->PackageName = TEXT("/Game/") + Row->PackageName;
    Row->ClassName = TEXT("Historical");
    Row->Storage = Storage;
    Row->Sync = EGameDepotSyncState::MissingLocal;
    Row->Severity = EGameDepotSeverity::Warning;
    Row->DesiredRule = (Storage == EGameDepotStorage::Git) ? TEXT("git") : TEXT("blob");
    Row->RemoteState = (Storage == EGameDepotStorage::OSS) ? TEXT("Exists in history") : TEXT("Git history");
    Row->Message = FString::Printf(TEXT("%s Size in latest historical manifest: %.2f MB."), *Message, static_cast<double>(SizeBytes) / (1024.0 * 1024.0));
    Row->ShortHash = FString::Printf(TEXT("%08x"), StableHash(DepotPath + TEXT("#history"))).Left(8);
    Row->bGitTracked = (Storage == EGameDepotStorage::Git);
    Row->bLocalExists = false;
    Row->bRemoteExists = (Storage == EGameDepotStorage::OSS);
    Row->bHistoryOnly = true;
    return Row;
}

void FGameDepotMockStatusProvider::UpsertRow(const FGameDepotAssetRowPtr& Row)
{
    if (!Row.IsValid())
    {
        return;
    }

    if (FGameDepotAssetRowPtr* Existing = RowByPath.Find(Row->DepotPath))
    {
        **Existing = *Row;
        return;
    }

    Rows.Add(Row);
    RowByPath.Add(Row->DepotPath, Row);
}

void FGameDepotMockStatusProvider::ApplyDeterministicMockState(FGameDepotAssetRow& Row) const
{
    const FString LowerPath = Row.DepotPath.ToLower();
    const uint32 H = StableHash(Row.DepotPath);
    Row.ShortHash = FString::Printf(TEXT("%08x"), H).Left(8);

    const bool bBinaryAsset = LowerPath.EndsWith(TEXT(".uasset")) || LowerPath.EndsWith(TEXT(".umap")) || LowerPath.EndsWith(TEXT(".wav")) || LowerPath.EndsWith(TEXT(".fbx")) || LowerPath.EndsWith(TEXT(".psd"));
    const bool bTextData = LowerPath.EndsWith(TEXT(".json")) || LowerPath.EndsWith(TEXT(".csv")) || LowerPath.EndsWith(TEXT(".ini")) || LowerPath.EndsWith(TEXT(".md"));
    const bool bUnknown = LowerPath.EndsWith(TEXT(".custom")) || LowerPath.Contains(TEXT("weird"));

    Row.bGitTracked = (H % 5 != 0);
    Row.bRemoteExists = (H % 7 != 0);
    Row.bLocalExists = (H % 17 != 0);

    if (bUnknown)
    {
        Row.Storage = EGameDepotStorage::Review;
        Row.Sync = EGameDepotSyncState::ReviewRequired;
        Row.Severity = EGameDepotSeverity::Warning;
        Row.DesiredRule = TEXT("review");
        Row.RemoteState = TEXT("Unknown");
        Row.Message = TEXT("Unknown file type. User must choose Git / OSS / Ignore before submit.");
        return;
    }

    if (bTextData)
    {
        Row.Storage = Row.bGitTracked ? EGameDepotStorage::Git : EGameDepotStorage::NewToGit;
        Row.DesiredRule = TEXT("git");
        Row.RemoteState = TEXT("N/A");
        Row.Sync = Row.bGitTracked ? ((H % 4 == 0) ? EGameDepotSyncState::Modified : EGameDepotSyncState::Synced) : EGameDepotSyncState::NewFile;
        Row.Severity = (Row.Sync == EGameDepotSyncState::Synced) ? EGameDepotSeverity::Good : EGameDepotSeverity::Info;
        Row.Message = Row.bGitTracked ? TEXT("Small text/data asset. Routed through Git in this mock version.") : TEXT("New text/data asset. Next submit would add it to Git.");
        return;
    }

    if (bBinaryAsset)
    {
        Row.Storage = Row.bGitTracked ? EGameDepotStorage::OSS : EGameDepotStorage::NewToOSS;
        Row.DesiredRule = TEXT("blob");
        Row.RemoteState = Row.bRemoteExists ? TEXT("Exists") : TEXT("Missing");
        if (!Row.bLocalExists)
        {
            Row.Sync = EGameDepotSyncState::MissingLocal;
            Row.Severity = EGameDepotSeverity::Error;
            Row.Message = TEXT("Local file is missing. Sync should restore it from OSS if the remote blob exists.");
        }
        else if (!Row.bRemoteExists)
        {
            Row.Sync = EGameDepotSyncState::MissingRemote;
            Row.Severity = EGameDepotSeverity::Error;
            Row.Message = TEXT("OSS blob is missing in mock remote. Upload or repair is required.");
        }
        else if (H % 6 == 0)
        {
            Row.Sync = EGameDepotSyncState::Modified;
            Row.Severity = EGameDepotSeverity::Warning;
            Row.Message = TEXT("Local content differs from manifest hash. Next submit would upload a new OSS blob.");
        }
        else
        {
            Row.Sync = Row.bGitTracked ? EGameDepotSyncState::Synced : EGameDepotSyncState::NewFile;
            Row.Severity = Row.bGitTracked ? EGameDepotSeverity::Good : EGameDepotSeverity::Info;
            Row.Message = Row.bGitTracked ? TEXT("Blob-managed asset is synced with mock manifest.") : TEXT("New binary asset. Next submit would upload it to OSS.");
        }
        return;
    }

    Row.Storage = EGameDepotStorage::Review;
    Row.Sync = EGameDepotSyncState::ReviewRequired;
    Row.Severity = EGameDepotSeverity::Warning;
    Row.DesiredRule = TEXT("review");
    Row.RemoteState = TEXT("Unknown");
    Row.Message = TEXT("No mock rule matched this file. Choose a rule from the context menu.");
}


void FGameDepotMockStatusProvider::RestoreDepotPathToHistory(const FString& DepotPath, const FGameDepotHistoryEntry& Entry)
{
    FGameDepotAssetRowPtr Row = FindByDepotPath(DepotPath);
    if (!Row.IsValid())
    {
        Row = MakeRowForDepotPath(DepotPath, true);
        UpsertRow(Row);
    }

    Row->bSelected = true;
    Row->Storage = Entry.Storage == EGameDepotStorage::Git ? EGameDepotStorage::Git : EGameDepotStorage::OSS;
    Row->DesiredRule = (Row->Storage == EGameDepotStorage::Git) ? TEXT("git") : TEXT("blob");
    Row->ShortHash = Entry.ShortHash;
    Row->bLocalExists = true;
    Row->bHistoryOnly = false;
    Row->ClassName = Row->ClassName == TEXT("Historical") ? TEXT("Restored Asset") : Row->ClassName;
    Row->bRemoteExists = Row->Storage == EGameDepotStorage::OSS ? true : Row->bRemoteExists;
    Row->RemoteState = Row->Storage == EGameDepotStorage::OSS ? TEXT("Exists") : TEXT("N/A");
    Row->Sync = EGameDepotSyncState::Modified;
    Row->Severity = EGameDepotSeverity::Warning;
    Row->Message = FString::Printf(TEXT("Restored mock history version %s. Submit is needed to commit this replacement."), *Entry.CommitId);
}
void FGameDepotMockStatusProvider::RevertUncommittedChanges(const TArray<FString>& DepotPaths)
{
    TSet<FString> NewFileRowsToRemove;

    for (const FString& DepotPath : DepotPaths)
    {
        FGameDepotAssetRowPtr Row = FindByDepotPath(DepotPath);
        if (!Row.IsValid())
        {
            continue;
        }

        Row->bSelected = true;

        if (Row->Sync == EGameDepotSyncState::NewFile || Row->Storage == EGameDepotStorage::NewToGit || Row->Storage == EGameDepotStorage::NewToOSS)
        {
            NewFileRowsToRemove.Add(DepotPath);
            continue;
        }

        if (Row->bHistoryOnly || Row->Sync == EGameDepotSyncState::MissingLocal)
        {
            Row->Severity = EGameDepotSeverity::Warning;
            Row->Message = TEXT("Mock revert skipped. This row is missing locally/history-only; use Restore from History instead.");
            continue;
        }

        Row->Sync = EGameDepotSyncState::Synced;
        Row->Severity = EGameDepotSeverity::Good;
        Row->Message = TEXT("Mock revert completed. Local uncommitted changes were reset to the latest committed version.");
        Row->RemoteState = (Row->Storage == EGameDepotStorage::OSS) ? (Row->bLocalBlobCached ? TEXT("Cached") : TEXT("Unknown")) : TEXT("N/A");
    }

    if (NewFileRowsToRemove.Num() > 0)
    {
        Rows.RemoveAll([&NewFileRowsToRemove](const FGameDepotAssetRowPtr& Row)
        {
            return Row.IsValid() && NewFileRowsToRemove.Contains(Row->DepotPath);
        });
        for (const FString& DepotPath : NewFileRowsToRemove)
        {
            RowByPath.Remove(DepotPath);
        }
    }
}

TArray<FGameDepotHistoryEntry> FGameDepotMockStatusProvider::BuildHistoryForDepotPath(const FString& DepotPath) const
{
    if (const TArray<FGameDepotHistoryEntry>* CachedHistory = HistoryByPath.Find(DepotPath))
    {
        return *CachedHistory;
    }

    TArray<FGameDepotHistoryEntry> Result;
    const FGameDepotAssetRowPtr Row = FindByDepotPath(DepotPath);
    const FString LowerPath = DepotPath.ToLower();
    const bool bGitLike = Row.IsValid()
        ? (Row->Storage == EGameDepotStorage::Git || Row->Storage == EGameDepotStorage::NewToGit)
        : (LowerPath.EndsWith(TEXT(".json")) || LowerPath.EndsWith(TEXT(".csv")) || LowerPath.EndsWith(TEXT(".ini")) || LowerPath.EndsWith(TEXT(".md")));

    const uint32 H = StableHash(DepotPath);
    const int64 BaseSize = static_cast<int64>((H % 4096) + 64) * 1024ll;

    for (int32 Index = 0; Index < 5; ++Index)
    {
        FGameDepotHistoryEntry Entry;
        Entry.CommitId = FString::Printf(TEXT("mock-%08x-%02d"), H ^ static_cast<uint32>(Index * 0x45d9f3bu), Index + 1);
        Entry.ShortHash = FString::Printf(TEXT("%08x"), H ^ static_cast<uint32>((Index + 1) * 0x9e3779b9u)).Left(8);
        Entry.Storage = bGitLike ? EGameDepotStorage::Git : EGameDepotStorage::OSS;
        Entry.SizeBytes = FMath::Max<int64>(256, BaseSize + static_cast<int64>(Index - 2) * 37ll * 1024ll);
        Entry.CommitDate = FDateTime::Now() - FTimespan::FromDays(static_cast<double>((Index + 1) * 3));
        Entry.Message = FString::Printf(
            TEXT("Mock historical %s version %d for %s."),
            bGitLike ? TEXT("Git") : TEXT("OSS/blob"),
            Index + 1,
            *DepotPath);
        Result.Add(Entry);
    }

    return Result;
}

uint32 FGameDepotMockStatusProvider::StableHash(const FString& Text)
{
    return FCrc::StrCrc32(*Text);
}


namespace
{
EGameDepotStorage GDStorageFromStrings(const FString& ManifestStorage, const FString& DesiredMode, const FString& Status, bool bHistoryOnly)
{
    const FString MS = ManifestStorage.ToLower();
    const FString DM = DesiredMode.ToLower();
    const FString ST = Status.ToLower();
    if (ST == TEXT("ignored") || DM == TEXT("ignore")) return EGameDepotStorage::Ignored;
    if (ST == TEXT("review") || DM == TEXT("review")) return EGameDepotStorage::Review;
    if (MS == TEXT("blob")) return EGameDepotStorage::OSS;
    if (MS == TEXT("git")) return EGameDepotStorage::Git;
    if (DM == TEXT("blob")) return EGameDepotStorage::NewToOSS;
    if (DM == TEXT("git")) return EGameDepotStorage::NewToGit;
    return bHistoryOnly ? EGameDepotStorage::OSS : EGameDepotStorage::Review;
}

EGameDepotSyncState GDSyncFromStatus(const FString& Status, const FString& DesiredMode)
{
    const FString S = Status.ToLower();
    if (S == TEXT("synced") || S == TEXT("present_unverified")) return EGameDepotSyncState::Synced;
    if (S == TEXT("modified")) return EGameDepotSyncState::Modified;
    if (S == TEXT("missing_local") || S == TEXT("missing_git_file")) return EGameDepotSyncState::MissingLocal;
    if (S == TEXT("missing_remote")) return EGameDepotSyncState::MissingRemote;
    if (S.Contains(TEXT("conflict"))) return EGameDepotSyncState::RoutingConflict;
    if (S == TEXT("new")) return EGameDepotSyncState::NewFile;
    if (S == TEXT("review")) return EGameDepotSyncState::ReviewRequired;
    if (S == TEXT("ignored")) return EGameDepotSyncState::Ignored;
    if (S == TEXT("history_only")) return EGameDepotSyncState::MissingLocal;
    if (DesiredMode.ToLower() == TEXT("review")) return EGameDepotSyncState::ReviewRequired;
    return EGameDepotSyncState::Modified;
}

EGameDepotSeverity GDSeverityFromString(const FString& Severity, EGameDepotSyncState Sync)
{
    const FString S = Severity.ToLower();
    if (S == TEXT("good") || S == TEXT("ok")) return EGameDepotSeverity::Good;
    if (S == TEXT("info")) return EGameDepotSeverity::Info;
    if (S == TEXT("error")) return EGameDepotSeverity::Error;
    if (S == TEXT("muted")) return EGameDepotSeverity::Muted;
    if (Sync == EGameDepotSyncState::Synced) return EGameDepotSeverity::Good;
    if (Sync == EGameDepotSyncState::MissingLocal || Sync == EGameDepotSyncState::MissingRemote || Sync == EGameDepotSyncState::RoutingConflict) return EGameDepotSeverity::Error;
    return EGameDepotSeverity::Warning;
}

FDateTime ParseDaemonDate(const FString& Text)
{
    FDateTime Date;
    if (FDateTime::ParseIso8601(*Text, Date)) return Date;
    if (FDateTime::Parse(Text, Date)) return Date;
    return FDateTime::Now();
}
}

bool FGameDepotMockStatusProvider::ReplaceRowsFromDaemonJSON(const FString& JsonText, FString& OutError)
{
    TSharedPtr<FJsonObject> Root;
    const TSharedRef<TJsonReader<>> Reader = TJsonReaderFactory<>::Create(JsonText);
    if (!FJsonSerializer::Deserialize(Reader, Root) || !Root.IsValid())
    {
        OutError = TEXT("Invalid JSON from /assets/status");
        return false;
    }
    const TArray<TSharedPtr<FJsonValue>>* Assets = nullptr;
    if (!Root->TryGetArrayField(TEXT("assets"), Assets) || Assets == nullptr)
    {
        OutError = TEXT("/assets/status response has no assets array");
        return false;
    }

    Rows.Reset();
    RowByPath.Reset();
    for (const TSharedPtr<FJsonValue>& Value : *Assets)
    {
        const TSharedPtr<FJsonObject> Obj = Value.IsValid() ? Value->AsObject() : nullptr;
        if (!Obj.IsValid()) continue;
        FGameDepotAssetRowPtr Row = MakeShared<FGameDepotAssetRow>();
        Obj->TryGetStringField(TEXT("path"), Row->DepotPath);
        Obj->TryGetStringField(TEXT("package"), Row->PackageName);
        if (Row->DepotPath.IsEmpty()) continue;
        Row->AssetName = FPaths::GetBaseFilename(Row->DepotPath);
        Row->ClassName = Row->PackageName.IsEmpty() ? TEXT("File") : TEXT("Asset");
        FString ManifestStorage, DesiredMode, Status, Severity;
        Obj->TryGetStringField(TEXT("manifest_storage"), ManifestStorage);
        Obj->TryGetStringField(TEXT("desired_mode"), DesiredMode);
        Obj->TryGetStringField(TEXT("status"), Status);
        Obj->TryGetStringField(TEXT("severity"), Severity);
        Row->DesiredRule = DesiredMode.IsEmpty() ? TEXT("-") : DesiredMode;
        Row->bHistoryOnly = false;
        Obj->TryGetBoolField(TEXT("history_only"), Row->bHistoryOnly);
        Row->Storage = GDStorageFromStrings(ManifestStorage, DesiredMode, Status, Row->bHistoryOnly);
        Row->Sync = GDSyncFromStatus(Status, DesiredMode);
        Row->Severity = GDSeverityFromString(Severity, Row->Sync);
        Obj->TryGetStringField(TEXT("message"), Row->Message);
        Obj->TryGetBoolField(TEXT("git_tracked"), Row->bGitTracked);
        const TSharedPtr<FJsonObject>* CurrentObj = nullptr;
        if (Obj->TryGetObjectField(TEXT("current"), CurrentObj) && CurrentObj && CurrentObj->IsValid())
        {
            FString SHA;
            (*CurrentObj)->TryGetStringField(TEXT("sha256"), SHA);
            Row->ShortHash = SHA.Left(8);
            (*CurrentObj)->TryGetBoolField(TEXT("local_exists"), Row->bLocalExists);
            (*CurrentObj)->TryGetBoolField(TEXT("remote_exists"), Row->bRemoteExists);
            (*CurrentObj)->TryGetBoolField(TEXT("remote_checked"), Row->bRemoteChecked);
            (*CurrentObj)->TryGetBoolField(TEXT("local_blob_cached"), Row->bLocalBlobCached);
            bool bBlobAvailable = false;
            if ((*CurrentObj)->TryGetBoolField(TEXT("blob_available"), bBlobAvailable) && bBlobAvailable)
            {
                Row->bRemoteExists = true;
            }
            if (Row->bLocalBlobCached)
            {
                Row->bRemoteExists = true;
            }
        }
        if (Row->ShortHash.IsEmpty()) Row->ShortHash = FString::Printf(TEXT("%08x"), static_cast<uint32>(StableHash(Row->DepotPath))).Left(8);
        Row->RemoteState = Row->Storage == EGameDepotStorage::OSS || Row->Storage == EGameDepotStorage::NewToOSS
            ? (Row->bLocalBlobCached ? TEXT("Cached") : (!Row->bRemoteChecked ? TEXT("Unknown") : (Row->bRemoteExists ? TEXT("Exists") : TEXT("Missing/Pending"))))
            : TEXT("N/A");
        UpsertRow(Row);
    }
    return true;
}

bool FGameDepotMockStatusProvider::SetHistoryFromDaemonJSON(const FString& DepotPath, const FString& JsonText, FString& OutError)
{
    TSharedPtr<FJsonObject> Root;
    const TSharedRef<TJsonReader<>> Reader = TJsonReaderFactory<>::Create(JsonText);
    if (!FJsonSerializer::Deserialize(Reader, Root) || !Root.IsValid())
    {
        OutError = TEXT("Invalid JSON from /assets/history");
        return false;
    }
    const TArray<TSharedPtr<FJsonValue>>* Versions = nullptr;
    if (!Root->TryGetArrayField(TEXT("versions"), Versions) || Versions == nullptr)
    {
        OutError = TEXT("/assets/history response has no versions array");
        return false;
    }
    TArray<FGameDepotHistoryEntry> RowsForPath;
    for (const TSharedPtr<FJsonValue>& Value : *Versions)
    {
        const TSharedPtr<FJsonObject> Obj = Value.IsValid() ? Value->AsObject() : nullptr;
        if (!Obj.IsValid()) continue;
        FGameDepotHistoryEntry Entry;
        Obj->TryGetStringField(TEXT("commit"), Entry.CommitId);
        Entry.ShortHash = Entry.CommitId.Left(8);
        Obj->TryGetStringField(TEXT("message"), Entry.Message);
        FString Storage, Date;
        Obj->TryGetStringField(TEXT("storage"), Storage);
        Obj->TryGetStringField(TEXT("sha256"), Entry.ShortHash);
        if (!Entry.ShortHash.IsEmpty()) Entry.ShortHash = Entry.ShortHash.Left(8);
        Entry.Storage = Storage.ToLower() == TEXT("blob") ? EGameDepotStorage::OSS : EGameDepotStorage::Git;
        double SizeNumber = 0.0;
        if (Obj->TryGetNumberField(TEXT("size"), SizeNumber)) Entry.SizeBytes = static_cast<int64>(SizeNumber);
        Obj->TryGetStringField(TEXT("date"), Date);
        Entry.CommitDate = ParseDaemonDate(Date);
        if (Entry.Message.IsEmpty()) Entry.Message = Storage.ToLower() == TEXT("blob") ? TEXT("OSS/blob version") : TEXT("Git version");
        RowsForPath.Add(Entry);
    }
    HistoryByPath.Add(DepotPath, RowsForPath);
    return true;
}
