// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {ERC20TransferModule} from "@datachainlab/cross-solidity/src/example/ERC20TransferModule.sol";
import {CrossContext} from "@datachainlab/cross-solidity/src/core/IContractModule.sol";
import {AuthType} from "@datachainlab/cross-solidity/src/proto/cross/core/auth/Auth.sol";

contract MyERC20TransferModule is ERC20TransferModule {
    bytes32 public constant EXTENSION_TYPE_URL_HASH = keccak256(abi.encodePacked("/verifier.sample.extension"));

    function _authorize(CrossContext calldata context, bytes calldata callInfo) internal view override {
        (address from,,) = decodeCallInfo(callInfo);

        bool authorized;
        uint256 len = context.signers.length;

        for (uint256 i = 0; i < len; ++i) {
            bytes calldata id = context.signers[i].id;

            if (id.length == 20 && address(uint160(bytes20(id))) == from) {
                if (context.signers[i].auth_type.mode != AuthType.AuthMode.AUTH_MODE_EXTENSION) {
                    continue;
                }
                if (
                    keccak256(abi.encodePacked(context.signers[i].auth_type.option.type_url)) != EXTENSION_TYPE_URL_HASH
                ) {
                    continue;
                }

                authorized = true;
                break;
            }
        }

        if (!authorized) {
            revert ERC20TransferModuleUnauthorized();
        }
    }
}
