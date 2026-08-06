"use client";

import { Modal, type ModalFuncProps } from "antd";
import { ExclamationCircleFilled } from "@ant-design/icons";

type ConfirmOptions = {
  title: string;
  content?: React.ReactNode;
  okText?: string;
  danger?: boolean;
  onConfirm: () => void | Promise<void>;
};

type ModalApi = { confirm: (config: ModalFuncProps) => void };

// Set by <ThemeBridge> (see ThemedApp.tsx) once antd's <App> context mounts,
// so the popup consumes the live ConfigProvider theme (dark/violet presets)
// instead of falling back to antd's static-function default look — which
// otherwise also logs "Static function can not consume context" to the
// console on every call. Null before mount (first paint) is harmless: the
// static Modal.confirm below still works, just unthemed for an instant.
let themedModal: ModalApi | null = null;
export function setThemedModal(api: ModalApi | null) {
  themedModal = api;
}

// Shared confirmation popup required before Logout, Save, Delete, Bulk
// Edit apply, Export, etc. (overview.md §6.0). One helper, one look and
// feel, called from anywhere instead of each screen rolling its own.
export function confirmAction({ title, content, okText = "Confirm", danger, onConfirm }: ConfirmOptions) {
  (themedModal ?? Modal).confirm({
    title,
    content,
    icon: <ExclamationCircleFilled />,
    okText,
    okButtonProps: danger ? { danger: true } : undefined,
    cancelText: "Cancel",
    onOk: onConfirm,
  });
}
