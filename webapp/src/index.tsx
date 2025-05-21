// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import type {Store, Action} from 'redux';
import React, {useState, useRef} from 'react';

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

export default class Plugin {
    public async initialize(registry: PluginRegistry, store: Store<GlobalState, Action<Record<string, unknown>>>) {
        // @ts-ignore - Ignore TypeScript errors for registerAdminConsoleCustomSetting
        registry.registerAdminConsoleCustomSetting('SyncUsers', (props: any) => {
            const [isLoading, setIsLoading] = useState(false);
            const [syncResult, setSyncResult] = useState<SyncResult | null>(null);
            const [showResults, setShowResults] = useState(false);
            
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
                        setShowResults(true);
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
            
            const toggleResults = (): void => {
                setShowResults(!showResults);
            };
            
            return (
                <div style={{width: '100%', maxWidth: '1200px'}}>
                    <button
                        className="btn btn-primary"
                        disabled={isLoading}
                        onClick={handleSync}
                        style={{
                            marginBottom: '20px', 
                            padding: '8px 16px',
                            fontSize: '14px'
                        }}
                    >
                        {isLoading ? 'Syncing...' : 'Sync Now'}
                    </button>
                    
                    {syncResult && (
                        <div style={{marginTop: '15px', marginBottom: '30px'}}>
                            <div 
                                className="panel panel-default" 
                                style={{
                                    border: '1px solid #ddd', 
                                    borderRadius: '4px',
                                    boxShadow: '0 1px 3px rgba(0,0,0,0.1)'
                                }}
                            >
                                <div 
                                    className="panel-heading" 
                                    style={{
                                        backgroundColor: '#f8f8f8', 
                                        padding: '12px 20px',
                                        borderBottom: '1px solid #ddd',
                                        cursor: 'pointer',
                                        display: 'flex',
                                        justifyContent: 'space-between',
                                        alignItems: 'center'
                                    }}
                                    onClick={toggleResults}
                                >
                                    <div style={{fontWeight: 'bold', fontSize: '15px'}}>
                                        <span style={{marginRight: '10px'}}>Sync Summary:</span>
                                        <span style={{
                                            display: 'inline-block', 
                                            marginRight: '15px', 
                                            color: syncResult.updated_count > 0 ? '#2389D7' : 'inherit'
                                        }}>
                                            Updated: {syncResult.updated_count}
                                        </span>
                                        <span style={{
                                            display: 'inline-block', 
                                            marginRight: '15px',
                                            color: syncResult.created_count > 0 ? '#26A970' : 'inherit'
                                        }}>
                                            Created: {syncResult.created_count}
                                        </span>
                                        <span style={{
                                            display: 'inline-block',
                                            color: syncResult.skipped_count > 0 ? '#FF8800' : 'inherit'
                                        }}>
                                            Skipped: {syncResult.skipped_count}
                                        </span>
                                    </div>
                                    <div style={{fontWeight: 'bold', color: '#555'}}>
                                        {showResults ? '▲ Hide Details' : '▼ Show Details'}
                                    </div>
                                </div>
                                
                                {showResults && (
                                    <div 
                                        className="panel-body"
                                        style={{
                                            padding: '15px 20px',
                                            backgroundColor: 'white'
                                        }}
                                    >
                                        <h4 style={{
                                            fontSize: '16px', 
                                            fontWeight: 'bold', 
                                            marginBottom: '12px',
                                            color: '#333'
                                        }}>
                                            Sync Details:
                                        </h4>
                                        <div style={{
                                            maxHeight: '400px', 
                                            overflowY: 'auto', 
                                            border: '1px solid #eee', 
                                            padding: '15px',
                                            borderRadius: '3px',
                                            backgroundColor: '#fafafa'
                                        }}>
                                            {syncResult.user_results.map((item: string, idx: number) => {
                                                // Determine the status from the text to apply appropriate styling
                                                let backgroundColor = '#f9f9f9';
                                                let borderLeftColor = '#ddd';
                                                let textColor = '#333';
                                                
                                                if (item.includes('Updated')) {
                                                    backgroundColor = '#EEF7FD';
                                                    borderLeftColor = '#2389D7';
                                                } else if (item.includes('Created')) {
                                                    backgroundColor = '#EEF9F2';
                                                    borderLeftColor = '#26A970';
                                                } else if (item.includes('Skipped')) {
                                                    backgroundColor = '#FFF8EE';
                                                    borderLeftColor = '#FF8800';
                                                }
                                                
                                                return (
                                                    <div 
                                                        key={idx} 
                                                        style={{
                                                            margin: '8px 0',
                                                            padding: '8px 12px',
                                                            backgroundColor,
                                                            borderLeft: `4px solid ${borderLeftColor}`,
                                                            borderRadius: '2px',
                                                            color: textColor,
                                                            fontSize: '14px',
                                                            lineHeight: '1.5'
                                                        }}
                                                    >
                                                        {item}
                                                    </div>
                                                );
                                            })}
                                        </div>
                                    </div>
                                )}
                            </div>
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