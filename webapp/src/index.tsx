// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import type {Store, Action} from 'redux';
import React, {useState} from 'react';

import type {GlobalState} from '@mattermost/types/store';

import manifest from './manifest';
import type {PluginRegistry} from './types/mattermost-webapp';

// Define SyncResult type to handle the response from API
interface SyncResult {
    matched_count: number;
    updated_count: number;
    created_count: number;
    skipped_count: number;
    user_results: string[];
}

// Define props for the modal component
interface SyncResultModalProps {
    result: SyncResult;
    close: () => void;
}

// Component to display sync results
const SyncResultModal: React.FC<SyncResultModalProps> = (props) => {
    const {result, close} = props;

    return (
        <div className="SyncResultModal modal-dialog">
            <div className="modal-content">
                <div className="modal-header">
                    <button
                        type="button"
                        className="close"
                        data-dismiss="modal"
                        aria-label="Close"
                        onClick={close}
                    >
                        <span aria-hidden="true">×</span>
                    </button>
                    <h4 className="modal-title">Sync Results</h4>
                </div>
                <div className="modal-body">
                    <div className="sync-summary">
                        <div className="sync-stats" style={{marginBottom: '20px', display: 'flex', justifyContent: 'space-between'}}>
                            <div className="sync-stat">
                                <span className="sync-stat-label">Already Mapped: </span>
                                <span className="sync-stat-value">{result.matched_count}</span>
                            </div>
                            <div className="sync-stat">
                                <span className="sync-stat-label">Updated: </span>
                                <span className="sync-stat-value">{result.updated_count}</span>
                            </div>
                            <div className="sync-stat">
                                <span className="sync-stat-label">Created: </span>
                                <span className="sync-stat-value">{result.created_count}</span>
                            </div>
                            <div className="sync-stat">
                                <span className="sync-stat-label">Skipped: </span>
                                <span className="sync-stat-value">{result.skipped_count}</span>
                            </div>
                        </div>
                        
                        <h4>Details:</h4>
                        <div className="sync-details" style={{maxHeight: '300px', overflowY: 'auto'}}>
                            {result.user_results.map((item: string, idx: number) => (
                                <div key={idx} className="sync-result-item" style={{margin: '5px 0', padding: '3px 0', borderBottom: '1px solid #eee'}}>
                                    {item}
                                </div>
                            ))}
                        </div>
                    </div>
                </div>
                <div className="modal-footer">
                    <button
                        type="button"
                        className="btn btn-primary"
                        onClick={close}
                    >
                        Close
                    </button>
                </div>
            </div>
        </div>
    );
};

export default class Plugin {
    public async initialize(registry: PluginRegistry, store: Store<GlobalState, Action<Record<string, unknown>>>) {
        // @ts-ignore - Ignore TypeScript errors for registerAdminConsoleCustomSetting
        registry.registerAdminConsoleCustomSetting('SyncUsers', (props: any) => {
            const [isLoading, setIsLoading] = useState(false);
            const [syncResult, setSyncResult] = useState<SyncResult | null>(null);
            const [showModal, setShowModal] = useState(false);
            
            const handleSync = async (): Promise<void> => {
                setIsLoading(true);
                
                try {
                    const response = await fetch(`/plugins/${manifest.id}/api/v1/sync`, {
                        method: 'POST',
                        headers: {
                            'Content-Type': 'application/json',
                            'X-Requested-With': 'XMLHttpRequest'
                        },
                        credentials: 'include'
                    });
                    
                    if (response.ok) {
                        const result = await response.json() as SyncResult;
                        setSyncResult(result);
                        setShowModal(true);
                    } else {
                        const errorText = await response.text();
                        console.error('Sync failed:', errorText);
                        alert(`Sync failed: ${response.status} ${response.statusText}\n${errorText}`);
                    }
                } catch (error) {
                    console.error('Error during sync:', error);
                    alert(`Error during sync: ${error instanceof Error ? error.message : 'Unknown error'}`);
                } finally {
                    setIsLoading(false);
                }
            };
            
            const closeModal = (): void => {
                setShowModal(false);
            };
            
            return (
                <div>
                    <button
                        className="btn btn-primary"
                        disabled={isLoading}
                        onClick={handleSync}
                    >
                        {isLoading ? 'Syncing...' : 'Sync Now'}
                    </button>
                    
                    {showModal && syncResult && (
                        <div className="modal fade in" style={{display: 'block'}}>
                            <SyncResultModal result={syncResult} close={closeModal} />
                            <div className="modal-backdrop fade in"></div>
                        </div>
                    )}
                </div>
            );
        });
    }
}

declare global {
    interface Window {
        registerPlugin(pluginId: string, plugin: Plugin): void;
    }
}

window.registerPlugin(manifest.id, new Plugin());