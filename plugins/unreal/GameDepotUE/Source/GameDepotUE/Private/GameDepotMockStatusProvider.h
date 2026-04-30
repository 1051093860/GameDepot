#pragma once

#include "CoreMinimal.h"
#include "AssetRegistry/AssetData.h"
#include "GameDepotStatusModel.h"

class FGameDepotMockStatusProvider : public TSharedFromThis<FGameDepotMockStatusProvider>
{
public:
    void RebuildFromAssetRegistry(int32 MaxAssets = 300);
    void AddOrUpdateAssets(const TArray<FAssetData>& Assets, bool bMarkSelected);
    void AddOrUpdateDepotPaths(const TArray<FString>& DepotPaths, bool bMarkSelected);
    void SetRuleForDepotPaths(const TArray<FString>& DepotPaths, const FString& Mode);
    void RevertUncommittedChanges(const TArray<FString>& DepotPaths);
    TArray<FGameDepotHistoryEntry> BuildHistoryForDepotPath(const FString& DepotPath) const;
    void RestoreDepotPathToHistory(const FString& DepotPath, const FGameDepotHistoryEntry& Entry);
    void AddMockHistoryOnlyRows();

    bool ReplaceRowsFromDaemonJSON(const FString& JsonText, FString& OutError);
    bool SetHistoryFromDaemonJSON(const FString& DepotPath, const FString& JsonText, FString& OutError);
    int32 CountHistoryOnly() const;
    void ClearSelectionMarks();

    const TArray<FGameDepotAssetRowPtr>& GetRows() const { return Rows; }
    FGameDepotAssetRowPtr FindByDepotPath(const FString& DepotPath) const;

    int32 CountByStorage(EGameDepotStorage Storage) const;
    int32 CountBySync(EGameDepotSyncState Sync) const;
    int32 CountBySeverity(EGameDepotSeverity Severity) const;

    static FString AssetDataToDepotPath(const FAssetData& AssetData);
    static FString ObjectPathToDepotPath(const FString& ObjectPath);

private:
    TArray<FGameDepotAssetRowPtr> Rows;
    TMap<FString, FGameDepotAssetRowPtr> RowByPath;
    TMap<FString, TArray<FGameDepotHistoryEntry>> HistoryByPath;

    FGameDepotAssetRowPtr MakeRowForAsset(const FAssetData& AssetData, bool bMarkSelected) const;
    FGameDepotAssetRowPtr MakeRowForDepotPath(const FString& DepotPath, bool bMarkSelected) const;
    void UpsertRow(const FGameDepotAssetRowPtr& Row);
    FGameDepotAssetRowPtr MakeHistoryOnlyRow(const FString& DepotPath, EGameDepotStorage Storage, int64 SizeBytes, const FString& Message) const;
    void ApplyDeterministicMockState(FGameDepotAssetRow& Row) const;
    static uint32 StableHash(const FString& Text);
};
