#include "SGameDepotHistoryDialog.h"

#include "GameDepotMockStatusProvider.h"
#include "Styling/AppStyle.h"
#include "Widgets/Input/SButton.h"
#include "Widgets/Layout/SBorder.h"
#include "Widgets/Layout/SSeparator.h"
#include "Widgets/Layout/SSpacer.h"
#include "Widgets/SBoxPanel.h"
#include "Widgets/SWindow.h"
#include "Widgets/Text/STextBlock.h"
#include "Widgets/Views/SHeaderRow.h"
#include "Widgets/Views/SListView.h"

#define LOCTEXT_NAMESPACE "SGameDepotHistoryDialog"

namespace
{
FText LocalStorageText(EGameDepotStorage Storage)
{
    switch (Storage)
    {
    case EGameDepotStorage::Git:
    case EGameDepotStorage::NewToGit:
        return LOCTEXT("StorageGit", "Git");
    case EGameDepotStorage::OSS:
    case EGameDepotStorage::NewToOSS:
        return LOCTEXT("StorageOSS", "OSS Blob");
    case EGameDepotStorage::Ignored:
        return LOCTEXT("StorageIgnore", "Ignore");
    default:
        return LOCTEXT("StorageReview", "Review");
    }
}

FText LocalSizeText(int64 SizeBytes)
{
    if (SizeBytes >= 1024ll * 1024ll * 1024ll)
    {
        return FText::FromString(FString::Printf(TEXT("%.2f GB"), static_cast<double>(SizeBytes) / static_cast<double>(1024ll * 1024ll * 1024ll)));
    }
    if (SizeBytes >= 1024ll * 1024ll)
    {
        return FText::FromString(FString::Printf(TEXT("%.2f MB"), static_cast<double>(SizeBytes) / static_cast<double>(1024ll * 1024ll)));
    }
    if (SizeBytes >= 1024ll)
    {
        return FText::FromString(FString::Printf(TEXT("%.1f KB"), static_cast<double>(SizeBytes) / 1024.0));
    }
    return FText::FromString(FString::Printf(TEXT("%lld B"), SizeBytes));
}

class SGameDepotHistoryRowWidget : public SMultiColumnTableRow<FGameDepotHistoryEntryPtr>
{
public:
    SLATE_BEGIN_ARGS(SGameDepotHistoryRowWidget) {}
        SLATE_ARGUMENT(FGameDepotHistoryEntryPtr, Item)
    SLATE_END_ARGS()

    void Construct(const FArguments& InArgs, const TSharedRef<STableViewBase>& OwnerTable)
    {
        Item = InArgs._Item;
        SMultiColumnTableRow<FGameDepotHistoryEntryPtr>::Construct(FSuperRowType::FArguments().Padding(2.0f), OwnerTable);
    }

    virtual TSharedRef<SWidget> GenerateWidgetForColumn(const FName& ColumnName) override
    {
        FString Text;
        if (Item.IsValid())
        {
            if (ColumnName == FName(TEXT("Commit"))) Text = Item->CommitId;
            else if (ColumnName == FName(TEXT("Storage"))) Text = LocalStorageText(Item->Storage).ToString();
            else if (ColumnName == FName(TEXT("Size"))) Text = LocalSizeText(Item->SizeBytes).ToString();
            else if (ColumnName == FName(TEXT("Date"))) Text = Item->CommitDate.ToString(TEXT("%Y-%m-%d %H:%M"));
            else if (ColumnName == FName(TEXT("Hash"))) Text = Item->ShortHash;
            else Text = Item->Message;
        }
        return SNew(STextBlock).Text(FText::FromString(Text)).ToolTipText(FText::FromString(Text));
    }

private:
    FGameDepotHistoryEntryPtr Item;
};
}

void SGameDepotHistoryDialog::Construct(const FArguments& InArgs)
{
    DepotPath = InArgs._DepotPath;
    Provider = InArgs._Provider;
    ParentWindow = InArgs._ParentWindow;
    OnRestored = InArgs._OnRestored;
    OnRestoreVersion = InArgs._OnRestoreVersion;

    if (Provider.IsValid())
    {
        const TArray<FGameDepotHistoryEntry> Generated = Provider->BuildHistoryForDepotPath(DepotPath);
        for (const FGameDepotHistoryEntry& Entry : Generated)
        {
            HistoryRows.Add(MakeShared<FGameDepotHistoryEntry>(Entry));
        }
        if (HistoryRows.Num() > 0)
        {
            SelectedEntry = HistoryRows[0];
        }
    }

    ChildSlot
    [
        SNew(SBorder)
        .BorderImage(FAppStyle::GetBrush("Brushes.Panel"))
        .Padding(12.0f)
        [
            SNew(SVerticalBox)
            + SVerticalBox::Slot().AutoHeight()[SNew(STextBlock).Text(LOCTEXT("Title", "GameDepot File History")).Font(FAppStyle::GetFontStyle("HeadingMedium"))]
            + SVerticalBox::Slot().AutoHeight().Padding(0, 6, 0, 8)[SNew(STextBlock).Text(FText::FromString(DepotPath)).AutoWrapText(true).ColorAndOpacity(FSlateColor(FLinearColor(0.70f, 0.78f, 0.90f)))]
            + SVerticalBox::Slot().AutoHeight().Padding(0, 0, 0, 8)[SNew(STextBlock).Text(LOCTEXT("Help", "History view: traverses Git commits and each commit manifest, then restores either Git object content or OSS blob content into the current workspace.")).AutoWrapText(true).ColorAndOpacity(FSlateColor(FLinearColor(0.62f, 0.62f, 0.62f)))]
            + SVerticalBox::Slot().FillHeight(1.0f)
            [
                SAssignNew(HistoryListView, SListView<FGameDepotHistoryEntryPtr>)
                .ListItemsSource(&HistoryRows)
                .SelectionMode(ESelectionMode::Single)
                .OnGenerateRow(this, &SGameDepotHistoryDialog::MakeHistoryRow)
                .OnSelectionChanged(this, &SGameDepotHistoryDialog::OnSelectionChanged)
                .HeaderRow
                (
                    SNew(SHeaderRow)
                    + SHeaderRow::Column(FName(TEXT("Commit"))).DefaultLabel(LOCTEXT("CommitHeader", "Commit")).FixedWidth(120.0f)
                    + SHeaderRow::Column(FName(TEXT("Storage"))).DefaultLabel(LOCTEXT("StorageHeader", "Storage")).FixedWidth(90.0f)
                    + SHeaderRow::Column(FName(TEXT("Size"))).DefaultLabel(LOCTEXT("SizeHeader", "Size")).FixedWidth(95.0f)
                    + SHeaderRow::Column(FName(TEXT("Date"))).DefaultLabel(LOCTEXT("DateHeader", "Commit Date")).FixedWidth(170.0f)
                    + SHeaderRow::Column(FName(TEXT("Hash"))).DefaultLabel(LOCTEXT("HashHeader", "Hash / Blob")).FixedWidth(95.0f)
                    + SHeaderRow::Column(FName(TEXT("Message"))).DefaultLabel(LOCTEXT("MessageHeader", "Message")).FillWidth(1.0f)
                )
            ]
            + SVerticalBox::Slot().AutoHeight().Padding(0, 8)[SNew(STextBlock).Text(this, &SGameDepotHistoryDialog::GetSelectedSummary).AutoWrapText(true).ColorAndOpacity(FSlateColor(FLinearColor(0.78f, 0.86f, 1.0f)))]
            + SVerticalBox::Slot().AutoHeight()[SNew(SSeparator)]
            + SVerticalBox::Slot().AutoHeight().Padding(0, 8, 0, 0)
            [
                SNew(SHorizontalBox)
                + SHorizontalBox::Slot().FillWidth(1.0f)[SNew(SSpacer)]
                + SHorizontalBox::Slot().AutoWidth().Padding(0, 0, 6, 0)[SNew(SButton).Text(LOCTEXT("Restore", "Restore Selected Version")).IsEnabled_Lambda([this]() { return SelectedEntry.IsValid(); }).OnClicked(this, &SGameDepotHistoryDialog::OnRestoreClicked)]
                + SHorizontalBox::Slot().AutoWidth()[SNew(SButton).Text(LOCTEXT("Cancel", "Cancel")).OnClicked(this, &SGameDepotHistoryDialog::OnCancelClicked)]
            ]
        ]
    ];

    if (HistoryListView.IsValid() && SelectedEntry.IsValid())
    {
        HistoryListView->SetSelection(SelectedEntry);
    }
}

TSharedRef<ITableRow> SGameDepotHistoryDialog::MakeHistoryRow(FGameDepotHistoryEntryPtr Item, const TSharedRef<STableViewBase>& OwnerTable)
{
    return SNew(SGameDepotHistoryRowWidget, OwnerTable).Item(Item);
}

void SGameDepotHistoryDialog::OnSelectionChanged(FGameDepotHistoryEntryPtr Item, ESelectInfo::Type SelectInfo)
{
    SelectedEntry = Item;
}

FReply SGameDepotHistoryDialog::OnRestoreClicked()
{
    if (SelectedEntry.IsValid())
    {
        if (OnRestoreVersion.IsBound())
        {
            OnRestoreVersion.Execute(DepotPath, *SelectedEntry);
        }
        else if (Provider.IsValid())
        {
            Provider->RestoreDepotPathToHistory(DepotPath, *SelectedEntry);
        }
        if (OnRestored.IsBound())
        {
            OnRestored.Execute();
        }
    }
    if (TSharedPtr<SWindow> Window = ParentWindow.Pin())
    {
        Window->RequestDestroyWindow();
    }
    return FReply::Handled();
}

FReply SGameDepotHistoryDialog::OnCancelClicked()
{
    if (TSharedPtr<SWindow> Window = ParentWindow.Pin())
    {
        Window->RequestDestroyWindow();
    }
    return FReply::Handled();
}

FText SGameDepotHistoryDialog::GetSelectedSummary() const
{
    if (!SelectedEntry.IsValid()) return LOCTEXT("NoSelection", "Select a version to restore.");
    return FText::FromString(FString::Printf(TEXT("Selected: %s | %s | %s | %s"), *SelectedEntry->CommitId, *StorageText(SelectedEntry->Storage).ToString(), *SizeText(SelectedEntry->SizeBytes).ToString(), *SelectedEntry->CommitDate.ToString(TEXT("%Y-%m-%d %H:%M"))));
}

FText SGameDepotHistoryDialog::StorageText(EGameDepotStorage Storage) { return LocalStorageText(Storage); }
FText SGameDepotHistoryDialog::SizeText(int64 SizeBytes) { return LocalSizeText(SizeBytes); }

#undef LOCTEXT_NAMESPACE
