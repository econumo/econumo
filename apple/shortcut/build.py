#!/usr/bin/env python3
"""Generate and sign the Apple Wallet shortcut.

Writes the unsigned XML plist source for the shortcut next to this file and,
unless --no-sign is given, signs them with `shortcuts sign --mode anyone` into
web/public/shortcuts/ — the served assets stay under web/, the builder does not.
Signing needs macOS with Shortcuts.app signed into an Apple ID; generating the
plists works anywhere with Python 3.

    python3 apple/shortcut/build.py            # plists + signed files
    python3 apple/shortcut/build.py --no-sign  # plists only

The served file name is the shortcut name (iOS takes it from the file), and
it carries no version: an updated recipe replaces the installed shortcut in
place, so the deep link and the user's automation keep pointing at it.
"""

import argparse
import plistlib
import shutil
import subprocess
import sys
import tempfile
import uuid
from pathlib import Path

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


def if_text_is(ref: dict, value: str, then: list) -> list:
    """If <ref> is <value> (condition 4, text comparison) ... End If, no Otherwise."""
    group = new_uuid()
    start = action(
        "conditional",
        GroupingIdentifier=group,
        WFControlFlowMode=0,
        WFCondition=4,
        WFConditionalActionString=value,
        WFInput={"Type": "Variable", "Variable": attachment(ref)},
    )
    end = action("conditional", GroupingIdentifier=group, WFControlFlowMode=2)
    return [start, *then, end]


def notification(*body) -> dict:
    return action("notification", WFNotificationActionBody=text(*body))


def stop() -> dict:
    return action("exit")


# ---- the shortcut -----------------------------------------------------------


def read_config() -> tuple[list, dict, dict]:
    """Get File + Dictionary + url + token: the actions, and the url / token refs."""
    file_uuid, dict_uuid, url_uuid, token_uuid = (new_uuid() for _ in range(4))
    file = output(file_uuid, "File")
    actions = [
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
    ]
    return actions, output(url_uuid, "Dictionary Value"), output(token_uuid, "Dictionary Value")


def post(url: dict, token: dict, body: dict) -> dict:
    return action(
        "downloadurl",
        UUID=new_uuid(),
        Advanced=True,
        ShowHeaders=True,
        WFURL=text(url, INGEST_PATH),
        WFHTTPMethod="POST",
        WFHTTPHeaders=dictionary({"Authorization": text("Bearer ", token)}),
        WFHTTPBodyType="JSON",
        WFJSONValues=dictionary(body),
    )


def setup_branch() -> list:
    """Run from the SPA's deep link with {url, token} as text input: save the
    configuration, then check in once so the run collects the server-host and
    iCloud Drive grants the unattended automation needs later."""
    dict_uuid, url_uuid, renamed_uuid = new_uuid(), new_uuid(), new_uuid()
    given_url = output(url_uuid, "Dictionary Value")
    # Read the saved file back rather than reusing the input: the grants are
    # tracked by data provenance, and the automation's request carries data
    # from that file.
    read, url, token = read_config()
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
            given_url,
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
        *read,
        # No card: the server stores it as failed with "account is required",
        # which the SPA takes as proof of contact and discards.
        post(url, token, {"check": text("setup")}),
        notification("Econumo configured for ", url),
        stop(),
    ]


def wallet_actions() -> list:
    type_uuid = new_uuid()
    setup = setup_branch()
    read, url, token = read_config()
    date_uuid = new_uuid()
    amount = "WFCurrencyAmountContentItem"
    return [
        # The setup deep link passes text; the Transaction automation passes a
        # wallet transaction; a run by hand passes nothing.
        action("getitemtype", UUID=type_uuid, WFInput=attachment(shortcut_input())),
        *if_text_is(output(type_uuid, "Type"), "Text", setup),
        *read,
        # The Transaction object carries no date; the automation fires within
        # seconds of the tap, so "now" on the device (with its offset) is it.
        action(
            "format.date",
            UUID=date_uuid,
            WFDate=text(current_date()),
            WFDateFormatStyle="ISO 8601",
            WFISO8601IncludeTime=True,
        ),
        post(
            url,
            token,
            {
                "account": text(shortcut_input(prop("Card or Pass"))),
                "payee": text(shortcut_input(prop("Merchant"))),
                "amount": text(shortcut_input(coerce(amount), prop("Currency Amount"))),
                "currency": text(shortcut_input(coerce(amount), prop("Currency Code"))),
                "occurredAt": text(output(date_uuid, "Formatted Date")),
                "type": text("expense"),
            },
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
        # 62322 is the dollar sign as today's icon picker stores it (read from
        # a Mac's Shortcuts.sqlite, ZSHORTCUTICON.ZGLYPHNUMBER). The share
        # preview (iCloud link, "Add Shortcut") shows the Files/Finder icon
        # instead for any shortcut with a Get File action, whatever the glyph;
        # the installed tile keeps the glyph and the colour.
        "WFWorkflowIcon": {"WFWorkflowIconGlyphNumber": 62322, "WFWorkflowIconStartColor": color},
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
    "econumo": (wallet_actions, 4274264319),  # yellow (0xFEC418FF)
}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--no-sign", action="store_true", help="only write the plists")
    args = parser.parse_args()

    for slug, (build, color) in SHORTCUTS.items():
        name = slug
        scope(slug)
        source = HERE / f"{slug}.plist"
        with source.open("wb") as fh:
            plistlib.dump(workflow(name, build(), color), fh, fmt=plistlib.FMT_XML, sort_keys=True)
        print(f"wrote {source.relative_to(HERE.parent.parent)}")
        if args.no_sign:
            continue
        PUBLIC.mkdir(parents=True, exist_ok=True)
        signed = PUBLIC / f"{name}.shortcut"
        # `shortcuts sign` refuses any input not named *.shortcut, and fails
        # ("The file doesn't exist") when the output path is already taken, so
        # it signs into a fresh directory and the result replaces the old file.
        # The directory sits next to this script: the signing service has been
        # seen to refuse inputs under the system temp folder.
        with tempfile.TemporaryDirectory(dir=HERE) as tmp:
            unsigned = Path(tmp) / "unsigned" / signed.name
            unsigned.parent.mkdir()
            unsigned.write_bytes(source.read_bytes())
            fresh = Path(tmp) / signed.name
            subprocess.run(
                ["shortcuts", "sign", "--mode", "anyone", "--input", str(unsigned), "--output", str(fresh)],
                check=True,
            )
            shutil.move(fresh, signed)
        print(f"signed {signed.relative_to(HERE.parent.parent)}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
