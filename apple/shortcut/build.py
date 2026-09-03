#!/usr/bin/env python3
"""Generate and sign the Apple Wallet shortcuts.

Writes the unsigned XML plist source for both shortcuts next to this file and,
unless --no-sign is given, signs them with `shortcuts sign --mode anyone` into
web/public/shortcuts/ — the served assets stay under web/, the builder does not.
Signing needs macOS with Shortcuts.app signed into an Apple ID; generating the
plists works anywhere with Python 3.

    python3 apple/shortcut/build.py            # plists + signed files
    python3 apple/shortcut/build.py --no-sign  # plists only

Bump VERSION when the recipe changes; the served file names and the
shortcut names (which iOS takes from the file name) carry it.
"""

import argparse
import plistlib
import subprocess
import sys
import tempfile
import uuid
from pathlib import Path

VERSION = 1
HERE = Path(__file__).resolve().parent
REPO_ROOT = HERE.parents[1]
PUBLIC = REPO_ROOT / "web" / "public" / "shortcuts"

CONFIG_FILE = "econumo-wallet.json"  # relative to iCloud Drive › Shortcuts
INGEST_PATH = "/api/v1/import/ingest-apple-wallet-event"
NOT_CONFIGURED = (
    "Econumo is not configured. Open Settings → Apple Wallet in Econumo and tap Configure."
)
INVALID_CONFIG = (
    "Invalid configuration. Open Settings → Apple Wallet in Econumo and tap Configure again."
)

PLACEHOLDER = "￼"  # U+FFFC marks a variable inside a token string


NAMESPACE = uuid.UUID("6f1c2a4e-9d3b-4c8f-a1e5-7b2d4f6a0000")
_scope = ""
_counter = 0


def new_uuid() -> str:
    """Deterministic per shortcut, so a rebuild without recipe changes is a no-op diff."""
    global _counter
    _counter += 1
    return str(uuid.uuid5(NAMESPACE, f"{_scope}:{_counter}")).upper()


def scope(slug: str) -> None:
    global _scope, _counter
    _scope, _counter = slug, 0


# ---- variable references ---------------------------------------------------


def shortcut_input(*aggrandizements: dict) -> dict:
    ref = {"Type": "ExtensionInput"}
    if aggrandizements:
        ref["Aggrandizements"] = list(aggrandizements)
    return ref


def output(action_uuid: str, name: str) -> dict:
    return {"Type": "ActionOutput", "OutputUUID": action_uuid, "OutputName": name}


def current_date() -> dict:
    return {"Type": "CurrentDate"}


def prop(name: str) -> dict:
    return {"Type": "WFPropertyVariableAggrandizement", "PropertyName": name}


def coerce(item_class: str) -> dict:
    return {"Type": "WFCoercionVariableAggrandizement", "CoercionItemClass": item_class}


def attachment(ref: dict) -> dict:
    """A parameter that IS a single variable (data-flow inputs: WFInput, WFDate...)."""
    return {"Value": ref, "WFSerializationType": "WFTextTokenAttachment"}


def text(*parts) -> dict:
    """A text parameter mixing literal strings and variable references."""
    string = ""
    attachments = {}
    for part in parts:
        if isinstance(part, str):
            string += part
        else:
            attachments[f"{{{len(string)}, 1}}"] = part
            string += PLACEHOLDER
    value = {"string": string}
    if attachments:
        value["attachmentsByRange"] = attachments
    return {"Value": value, "WFSerializationType": "WFTextTokenString"}


def dictionary(items: dict) -> dict:
    """Headers / JSON body: every value is a text field (WFItemType 0)."""
    return {
        "Value": {
            "WFDictionaryFieldValueItems": [
                {"WFItemType": 0, "WFKey": text(key), "WFValue": value}
                for key, value in items.items()
            ]
        },
        "WFSerializationType": "WFDictionaryFieldValue",
    }


# ---- actions ----------------------------------------------------------------


def action(identifier: str, **params) -> dict:
    return {
        "WFWorkflowActionIdentifier": f"is.workflow.actions.{identifier}",
        "WFWorkflowActionParameters": params,
    }


def if_has_no_value(ref: dict, then: list) -> list:
    """If <ref> does not have any value (condition 101) ... End If, no Otherwise."""
    group = new_uuid()
    start = action(
        "conditional",
        GroupingIdentifier=group,
        WFControlFlowMode=0,
        WFCondition=101,
        WFInput={"Type": "Variable", "Variable": attachment(ref)},
    )
    end = action("conditional", GroupingIdentifier=group, WFControlFlowMode=2)
    return [start, *then, end]


def notification(*body) -> dict:
    return action("notification", WFNotificationActionBody=text(*body))


def stop() -> dict:
    return action("exit")


# ---- the two shortcuts ------------------------------------------------------


def setup_actions() -> list:
    dict_uuid, url_uuid, renamed_uuid = new_uuid(), new_uuid(), new_uuid()
    url = output(url_uuid, "Dictionary Value")
    return [
        action("detect.dictionary", UUID=dict_uuid, WFInput=attachment(shortcut_input())),
        action(
            "getvalueforkey",
            UUID=url_uuid,
            WFInput=attachment(output(dict_uuid, "Dictionary")),
            WFDictionaryKey="url",
            WFGetDictionaryValueType="Value",
        ),
        *if_has_no_value(
            url,
            [
                action(
                    "alert",
                    WFAlertActionTitle=text("Econumo"),
                    WFAlertActionMessage=text(INVALID_CONFIG),
                    WFAlertActionCancelButtonShown=False,
                ),
                stop(),
            ],
        ),
        action(
            "setitemname",
            UUID=renamed_uuid,
            WFInput=attachment(shortcut_input()),
            WFName=CONFIG_FILE,
            WFDontIncludeFileExtension=False,
        ),
        action(
            "documentpicker.save",
            WFInput=attachment(output(renamed_uuid, "Renamed Item")),
            WFAskWhereToSave=False,
            WFFileDestinationPath=CONFIG_FILE,
            WFSaveFileOverwrite=True,
        ),
        notification("Econumo configured for ", url),
    ]


def wallet_actions() -> list:
    file_uuid, dict_uuid, url_uuid, token_uuid, date_uuid = (new_uuid() for _ in range(5))
    file = output(file_uuid, "File")
    url = output(url_uuid, "Dictionary Value")
    token = output(token_uuid, "Dictionary Value")
    amount = "WFCurrencyAmountContentItem"
    return [
        action(
            "documentpicker.open",
            UUID=file_uuid,
            WFGetFilePath=CONFIG_FILE,
            WFShowFilePicker=False,
            WFFileErrorIfNotFound=False,
        ),
        *if_has_no_value(file, [notification(NOT_CONFIGURED), stop()]),
        action("detect.dictionary", UUID=dict_uuid, WFInput=attachment(file)),
        action(
            "getvalueforkey",
            UUID=url_uuid,
            WFInput=attachment(output(dict_uuid, "Dictionary")),
            WFDictionaryKey="url",
            WFGetDictionaryValueType="Value",
        ),
        action(
            "getvalueforkey",
            UUID=token_uuid,
            WFInput=attachment(output(dict_uuid, "Dictionary")),
            WFDictionaryKey="token",
            WFGetDictionaryValueType="Value",
        ),
        # The Transaction object carries no date; the automation fires within
        # seconds of the tap, so "now" on the device (with its offset) is it.
        action(
            "format.date",
            UUID=date_uuid,
            WFDate=text(current_date()),
            WFDateFormatStyle="ISO 8601",
            WFISO8601IncludeTime=True,
        ),
        action(
            "downloadurl",
            UUID=new_uuid(),
            Advanced=True,
            ShowHeaders=True,
            WFURL=text(url, INGEST_PATH),
            WFHTTPMethod="POST",
            WFHTTPHeaders=dictionary({"Authorization": text("Bearer ", token)}),
            WFHTTPBodyType="JSON",
            WFJSONValues=dictionary(
                {
                    "account": text(shortcut_input(prop("Card or Pass"))),
                    "payee": text(shortcut_input(prop("Merchant"))),
                    "amount": text(shortcut_input(coerce(amount), prop("Currency Amount"))),
                    "currency": text(shortcut_input(coerce(amount), prop("Currency Code"))),
                    "occurredAt": text(output(date_uuid, "Formatted Date")),
                    "type": text("expense"),
                }
            ),
        ),
    ]


INPUT_CLASSES = [
    "WFAppStoreAppContentItem",
    "WFArticleContentItem",
    "WFContactContentItem",
    "WFDateContentItem",
    "WFEmailAddressContentItem",
    "WFGenericFileContentItem",
    "WFImageContentItem",
    "WFiTunesProductContentItem",
    "WFLocationContentItem",
    "WFDCMapsLinkContentItem",
    "WFAVAssetContentItem",
    "WFPDFContentItem",
    "WFPhoneNumberContentItem",
    "WFRichTextContentItem",
    "WFSafariWebPageContentItem",
    "WFStringContentItem",
    "WFURLContentItem",
]


def workflow(name: str, actions: list, color: int) -> dict:
    return {
        "WFWorkflowName": name,
        "WFWorkflowClientVersion": "2700.0.4",
        "WFWorkflowMinimumClientVersion": 900,
        "WFWorkflowMinimumClientVersionString": "900",
        "WFWorkflowIcon": {"WFWorkflowIconGlyphNumber": 59395, "WFWorkflowIconStartColor": color},
        "WFWorkflowHasShortcutInputVariables": True,
        "WFWorkflowHasOutputFallback": False,
        "WFWorkflowImportQuestions": [],
        "WFWorkflowInputContentItemClasses": INPUT_CLASSES,
        "WFWorkflowOutputContentItemClasses": [],
        "WFWorkflowTypes": [],
        "WFWorkflowActions": actions,
    }


# iOS names an imported shortcut after the FILE it came from, ignoring
# WFWorkflowName, so the display name must equal the served file's basename:
# the deep link in the SPA and the automation instructions address it by
# that name.
SHORTCUTS = {
    "econumo-setup": (setup_actions, 463140863),  # blue
    "econumo-wallet": (wallet_actions, 4292093695),  # green
}


def shortcut_name(slug: str) -> str:
    return f"{slug}-v{VERSION}"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--no-sign", action="store_true", help="only write the plists")
    args = parser.parse_args()

    for slug, (build, color) in SHORTCUTS.items():
        name = shortcut_name(slug)
        scope(slug)
        source = HERE / f"{slug}.plist"
        with source.open("wb") as fh:
            plistlib.dump(workflow(name, build(), color), fh, fmt=plistlib.FMT_XML, sort_keys=True)
        print(f"wrote {source.relative_to(HERE.parent.parent)}")
        if args.no_sign:
            continue
        PUBLIC.mkdir(parents=True, exist_ok=True)
        signed = PUBLIC / f"{name}.shortcut"
        # `shortcuts sign` refuses any input not named *.shortcut.
        with tempfile.TemporaryDirectory() as tmp:
            unsigned = Path(tmp) / signed.name
            unsigned.write_bytes(source.read_bytes())
            subprocess.run(
                ["shortcuts", "sign", "--mode", "anyone", "--input", str(unsigned), "--output", str(signed)],
                check=True,
            )
        print(f"signed {signed.relative_to(HERE.parent.parent)}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
